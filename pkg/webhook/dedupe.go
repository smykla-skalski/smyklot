package webhook

import (
	"sync"
	"time"
)

// Default sizing for a Deduper.
const (
	// DefaultTTL is how long a key is remembered. GitHub gives up redelivering
	// long before this, and holding keys longer only costs memory
	DefaultTTL = time.Hour

	// DefaultMaxEntries caps memory when deliveries arrive faster than they
	// expire
	DefaultMaxEntries = 10000
)

// Deduper remembers which events have already been handled.
//
// A key is claimed before the work starts rather than after it finishes, so two
// copies of one delivery arriving at once cannot both run. Work that fails
// releases its key through Abandon, leaving a redelivery free to retry - the
// alternative would turn one failure into permanent silence for that comment.
//
// Claims live in memory, so a redelivery that arrives after a restart runs
// again.
type Deduper struct {
	ttl  time.Duration
	max  int
	now  func() time.Time
	mu   sync.Mutex
	seen map[string]time.Time
}

// NewDeduper creates a Deduper.
//
// A non-positive ttl or maxEntries falls back to the package default. A nil now
// uses the wall clock; tests pass a fake one so expiry can be checked without
// waiting for it.
func NewDeduper(ttl time.Duration, maxEntries int, now func() time.Time) *Deduper {
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}

	if now == nil {
		now = time.Now
	}

	return &Deduper{
		ttl:  ttl,
		max:  maxEntries,
		now:  now,
		seen: make(map[string]time.Time),
	}
}

// Begin claims a key, reporting whether the caller may proceed.
//
// A false return means the key is already claimed and the caller must do
// nothing.
func (d *Deduper) Begin(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()

	if claimed, ok := d.seen[key]; ok && now.Sub(claimed) < d.ttl {
		return false
	}

	d.evict(now)
	d.seen[key] = now

	return true
}

// Abandon releases a claim so a redelivery can retry the work.
func (d *Deduper) Abandon(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.seen, key)
}

// evict makes room for one more key, dropping expired claims first and the
// oldest surviving claim only if that was not enough.
//
// Callers must hold the mutex.
func (d *Deduper) evict(now time.Time) {
	if len(d.seen) < d.max {
		return
	}

	for key, claimed := range d.seen {
		if now.Sub(claimed) >= d.ttl {
			delete(d.seen, key)
		}
	}

	if len(d.seen) < d.max {
		return
	}

	var (
		oldestKey string
		oldest    time.Time
	)

	for key, claimed := range d.seen {
		if oldestKey == "" || claimed.Before(oldest) {
			oldestKey, oldest = key, claimed
		}
	}

	delete(d.seen, oldestKey)
}
