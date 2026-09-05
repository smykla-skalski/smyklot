package sqlstore

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// queryStatsLimit caps how many distinct statements are counted.
//
// The name comes from the function that issued the statement, so the set is
// bounded by the source and cannot be grown by traffic. The cap is a floor
// under that reasoning rather than a live concern: a future package with more
// query functions than this loses the tail, and does not lose the process.
const queryStatsLimit = 512

// queryStats counts what the store's own statements cost.
//
// It is deliberately a counter rather than a log. One row per statement would
// be the same unbounded ledger the queue already learned not to keep, and the
// question an operator asks - is something slower than it was - is answered by
// a mean and a worst case per hour.
type queryStats struct {
	mu      sync.Mutex
	byName  map[string]*queryStat
	dropped int64
}

type queryStat struct {
	observations int64
	failures     int64
	total        time.Duration
	max          time.Duration
}

func newQueryStats() *queryStats {
	return &queryStats{byName: map[string]*queryStat{}}
}

func (s *queryStats) observe(name string, elapsed time.Duration, failed bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stat, found := s.byName[name]
	if !found {
		if len(s.byName) >= queryStatsLimit {
			s.dropped++

			return
		}
		stat = &queryStat{}
		s.byName[name] = stat
	}
	stat.observations++
	stat.total += elapsed
	if elapsed > stat.max {
		stat.max = elapsed
	}
	if failed {
		stat.failures++
	}
}

// drain returns what has been counted and starts again.
//
// Resetting is what makes each sample an hour of its own rather than a running
// total the reader has to difference, and it means a process that dies loses
// at most the hour it was in.
func (s *queryStats) drain() []storage.QueryStats {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	drained := make([]storage.QueryStats, 0, len(s.byName))
	for name, stat := range s.byName {
		drained = append(drained, storage.QueryStats{
			Name: name, Observations: stat.observations, Failures: stat.failures,
			Total: stat.total, Max: stat.max,
		})
	}
	s.byName = map[string]*queryStat{}

	return drained
}

// callerNames caches the name of the function at a program counter, because
// resolving one costs more than the statement it is naming.
var callerNames sync.Map

// queryCaller names the function that issued a statement.
//
// Nothing passes a name in. Every statement in this package is written inside
// the function that owns it, so the call stack already carries a better label
// than a string a caller would have to remember to change.
//
// It searches the stack rather than counting frames off it. The statement
// surface is reached through an embedded binder, so whether a promoted-method
// wrapper stands between the caller and the query - and whether the compiler
// inlined it - decides the depth, and a fixed count named the test that opened
// the store instead of the function that wrote the SQL. The first frame that
// is neither the plumbing nor another package is the answer at any depth.
//
// The captured counters together identify the call path, so the search runs
// once per path and every later statement along it reads the cache. They are
// hashed rather than keyed on the innermost one, which is inside the binder
// and therefore identical for every caller in the package - keying on it gave
// every query in the process the name of the first one to run.
func queryCaller(skip int) string {
	var pcs [8]uintptr
	captured := runtime.Callers(skip, pcs[:])
	if captured < 1 {
		return unknownQuery
	}
	key := callPathKey(pcs[:captured])
	if cached, found := callerNames.Load(key); found {
		if name, ok := cached.(string); ok {
			return name
		}
	}
	name := searchQueryCaller(pcs[:captured])
	callerNames.Store(key, name)

	return name
}

// callPathKey folds a captured stack into one comparable value, so the cache
// is keyed by the whole path rather than by any single frame in it.
func callPathKey(pcs []uintptr) uint64 {
	const offset, prime = uint64(14695981039346656037), uint64(1099511628211)
	key := offset
	for _, pc := range pcs {
		for shift := 0; shift < 64; shift += 8 {
			key = (key ^ (uint64(pc)>>shift)&0xff) * prime
		}
	}

	return key
}

const (
	unknownQuery = "unknown"
	storePackage = "internal/storage/sqlstore."
)

// queryPlumbing is the statement surface itself, which never names a query.
var queryPlumbing = []string{"binder.", "handle.", "transaction.", "queryCaller"}

func searchQueryCaller(pcs []uintptr) string {
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, storePackage) && !isQueryPlumbing(frame.Function) {
			return shortQueryName(frame.Function)
		}
		if !more {
			return unknownQuery
		}
	}
}

func isQueryPlumbing(function string) bool {
	short := shortQueryName(function)
	for _, plumbing := range queryPlumbing {
		if strings.HasPrefix(short, plumbing) || short == plumbing {
			return true
		}
	}

	return false
}

func shortQueryName(function string) string {
	if function == "" {
		return unknownQuery
	}
	if index := strings.LastIndex(function, "/"); index >= 0 {
		function = function[index+1:]
	}
	if index := strings.Index(function, "."); index >= 0 {
		function = function[index+1:]
	}

	return strings.NewReplacer("(", "", ")", "", "*", "").Replace(function)
}
