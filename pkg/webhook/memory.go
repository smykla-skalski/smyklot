package webhook

import (
	"context"
	"sync"
	"time"
)

// Defaults for a MemoryInbox.
const (
	// DefaultTTL is how long a settled delivery is remembered. GitHub gives up
	// redelivering long before this, and holding keys longer only costs memory.
	DefaultTTL = time.Hour

	// DefaultMaxEntries caps memory when deliveries arrive faster than they
	// expire.
	DefaultMaxEntries = 10000
)

// MemoryInboxOptions sizes a MemoryInbox. Every zero value is a working
// default.
type MemoryInboxOptions struct {
	TTL        time.Duration
	MaxEntries int

	// Now is the clock. Tests pass a fake one so expiry and backoff can be
	// checked without waiting for them.
	Now func() time.Time
}

// MemoryInbox is an Inbox that keeps everything in memory.
//
// A restart loses every claim, so a delivery that was in flight is neither run
// nor redelivered - which is the failure a durable inbox exists to prevent, and
// the reason this is not the default. It is here for two callers: an App small
// enough that a lost delivery is a shrug, and this package's own tests, which
// exercise the whole claim-lease-retry loop without a database.
type MemoryInbox struct {
	ttl time.Duration
	max int
	now func() time.Time

	mu     sync.Mutex
	nextID int64
	rows   map[int64]*memoryRow
	byKey  map[string]*memoryRow
}

type memoryRow struct {
	id        int64
	key       string
	work      Work
	claimedAt time.Time

	// settledAt is zero while the delivery is still live. A settled row is kept
	// so a redelivery is recognised rather than run again, and dropped once its
	// TTL is up.
	settledAt time.Time

	// forgotten marks a retryable failure: the delivery is over, but GitHub is
	// welcome to send it again.
	forgotten bool

	nextAttemptAt time.Time
	leaseUntil    time.Time
}

// NewMemoryInbox returns an in-memory Inbox. See MemoryInbox for what that
// costs.
func NewMemoryInbox(opts MemoryInboxOptions) *MemoryInbox {
	if opts.TTL <= 0 {
		opts.TTL = DefaultTTL
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = DefaultMaxEntries
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}

	return &MemoryInbox{
		ttl: opts.TTL, max: opts.MaxEntries, now: opts.Now,
		rows: make(map[int64]*memoryRow), byKey: make(map[string]*memoryRow),
	}
}

func (m *MemoryInbox) Claim(_ context.Context, claim Claim) (ClaimResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if existing, ok := m.byKey[claim.Key]; ok && !m.expired(existing, now) {
		switch {
		case existing.settledAt.IsZero():
			return ClaimResult{Disposition: InProgress}, nil
		case existing.forgotten:
			// A retryable failure is not retained: this is GitHub trying
			// again, which is exactly what "retryable" meant.
			delete(m.byKey, existing.key)
			delete(m.rows, existing.id)
		default:
			return ClaimResult{Disposition: Retained}, nil
		}
	}

	m.evict(now)
	m.nextID++
	row := &memoryRow{
		id:  m.nextID,
		key: claim.Key,
		work: Work{
			ClaimID: m.nextID, Key: claim.Key, DeliveryID: claim.DeliveryID,
			Event: claim.Event, Payload: claim.Payload,
		},
		claimedAt:     now,
		nextAttemptAt: now,
	}
	m.rows[row.id] = row
	m.byKey[row.key] = row

	return ClaimResult{ID: row.id, Disposition: Accepted}, nil
}

func (m *MemoryInbox) Lease(_ context.Context, now, leaseExpiresAt time.Time) (Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var (
		ready   *memoryRow
		soonest *time.Time
	)
	for _, row := range m.rows {
		if !row.settledAt.IsZero() {
			continue
		}
		at := row.nextAttemptAt
		if row.leaseUntil.After(at) {
			at = row.leaseUntil
		}
		if at.After(now) {
			if soonest == nil || at.Before(*soonest) {
				when := at
				soonest = &when
			}

			continue
		}
		// Oldest first, so a delivery cannot be starved by newer arrivals.
		if ready == nil || row.id < ready.id {
			ready = row
		}
	}
	if ready == nil {
		return Lease{AvailableAt: soonest}, nil
	}

	ready.leaseUntil = leaseExpiresAt
	ready.work.Attempt++
	work := ready.work

	return Lease{Work: &work}, nil
}

func (m *MemoryInbox) Complete(_ context.Context, claimID int64, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if row, ok := m.rows[claimID]; ok && row.settledAt.IsZero() {
		row.settledAt = at
	}

	return nil
}

func (m *MemoryInbox) Fail(_ context.Context, failure Failure) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if row, ok := m.rows[failure.ClaimID]; ok && row.settledAt.IsZero() {
		row.settledAt = failure.At
		row.forgotten = failure.Retryable
	}

	return nil
}

func (m *MemoryInbox) Retry(_ context.Context, retry Retry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if row, ok := m.rows[retry.ClaimID]; ok && row.settledAt.IsZero() {
		row.nextAttemptAt = retry.At
		row.leaseUntil = time.Time{}
	}

	return nil
}

// expired reports whether a settled row has outlived its TTL.
func (m *MemoryInbox) expired(row *memoryRow, now time.Time) bool {
	return !row.settledAt.IsZero() && now.Sub(row.settledAt) >= m.ttl
}

// evict makes room for one more delivery, dropping settled rows past their TTL
// first and the oldest settled row only if that was not enough.
//
// A live delivery is never dropped: forgetting one would let a redelivery run
// beside the copy still executing, which is the whole thing the claim prevents.
//
// Callers must hold the mutex.
func (m *MemoryInbox) evict(now time.Time) {
	if len(m.rows) < m.max {
		return
	}

	for id, row := range m.rows {
		if m.expired(row, now) {
			delete(m.rows, id)
			delete(m.byKey, row.key)
		}
	}
	if len(m.rows) < m.max {
		return
	}

	var oldest *memoryRow
	for _, row := range m.rows {
		if row.settledAt.IsZero() {
			continue
		}
		if oldest == nil || row.settledAt.Before(oldest.settledAt) {
			oldest = row
		}
	}
	if oldest != nil {
		delete(m.rows, oldest.id)
		delete(m.byKey, oldest.key)
	}
}
