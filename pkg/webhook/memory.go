package webhook

import (
	"context"
	"sync"
	"time"
)

const (
	DefaultTTL        = time.Hour
	DefaultMaxEntries = 10000
)

type MemoryInboxOptions struct {
	TTL        time.Duration
	MaxEntries int
	Now        func() time.Time
}

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
	settledAt time.Time
	forgotten bool

	nextAttemptAt time.Time
	leaseUntil    time.Time
}

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

func (m *MemoryInbox) expired(row *memoryRow, now time.Time) bool {
	return !row.settledAt.IsZero() && now.Sub(row.settledAt) >= m.ttl
}

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
