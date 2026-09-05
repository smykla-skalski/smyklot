package sqlstore

import (
	"testing"
	"time"
)

func BenchmarkQueryCaller(b *testing.B) {
	for b.Loop() {
		_ = queryCaller()
	}
}

func BenchmarkQueryStatsObserve(b *testing.B) {
	stats := newQueryStats()
	for b.Loop() {
		stats.observe("Store.ListWorkQueue", time.Millisecond, false)
	}
}

func BenchmarkQueryStatsObserveParallel(b *testing.B) {
	stats := newQueryStats()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats.observe(queryCaller(), time.Millisecond, false)
		}
	})
}
