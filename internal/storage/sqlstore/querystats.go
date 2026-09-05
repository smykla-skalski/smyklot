package sqlstore

import (
	"hash/maphash"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

var callPathSeed = maphash.MakeSeed()

const queryCallerSkip = 2

type queryStats struct {
	mu     sync.Mutex
	byName map[string]*queryStat
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

var callerNames sync.Map

func queryCaller() string {
	var pcs [8]uintptr
	captured := runtime.Callers(queryCallerSkip, pcs[:])
	if captured < 1 {
		return unknownQuery
	}
	key := maphash.Comparable(callPathSeed, pcs)
	if cached, found := callerNames.Load(key); found {
		if name, ok := cached.(string); ok {
			return name
		}
	}

	return resolveQueryCaller(key, pcs, captured)
}

//go:noinline
func resolveQueryCaller(key uint64, pcs [8]uintptr, captured int) string {
	name := searchQueryCaller(pcs[:captured])
	callerNames.Store(key, name)

	return name
}

const (
	unknownQuery = "unknown"
	storePackage = "internal/storage/sqlstore."
)

var queryPlumbing = []string{"binder.", "handle.", "transaction.", "queryCaller", "writeEach"}

func searchQueryCaller(pcs []uintptr) string {
	frames := runtime.CallersFrames(pcs)
	owner, caller := "", ""
	for more := true; more; {
		var frame runtime.Frame
		frame, more = frames.Next()
		if !strings.Contains(frame.Function, storePackage) || isQueryPlumbing(frame.Function) {
			continue
		}
		if owner == "" {
			owner = shortQueryName(frame.Function)

			continue
		}
		caller = shortQueryName(frame.Function)

		break
	}
	switch {
	case owner == "":
		return unknownQuery
	case caller == "":
		return owner
	default:
		return caller + "." + owner
	}
}

func isQueryPlumbing(function string) bool {
	short := shortQueryName(function)
	for _, plumbing := range queryPlumbing {
		if strings.HasPrefix(short, plumbing) {
			return true
		}
	}

	return false
}

func shortQueryName(function string) string {
	if function == "" {
		return unknownQuery
	}
	function = withoutTypeArguments(function)
	if index := strings.LastIndex(function, "/"); index >= 0 {
		function = function[index+1:]
	}
	if index := strings.Index(function, "."); index >= 0 {
		function = function[index+1:]
	}

	return strings.NewReplacer("(", "", ")", "", "*", "").Replace(function)
}

func withoutTypeArguments(function string) string {
	opened := strings.Index(function, "[")
	closed := strings.LastIndex(function, "]")
	if opened < 0 || closed < opened {
		return function
	}

	return function[:opened] + function[closed+1:]
}
