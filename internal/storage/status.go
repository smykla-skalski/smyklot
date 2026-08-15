package storage

import "time"

// DatabaseStatus is what the storage subsystem reports about itself.
//
// Every field here is a value to be read, not a fact to branch on. Engine
// names the database the way Version names its release, so a caller prints it
// and nothing more - which is what keeps a third engine a change inside
// internal/storage rather than one that reaches the panel.
type DatabaseStatus struct {
	// Engine names the database for a reader: "PostgreSQL", not "postgres".
	Engine string

	// Version is the server's own release, without whatever packaging string
	// it reports beside it. Empty when it could not be read.
	Version string

	// SchemaVersion is the highest migration the database has applied. A
	// service that started has applied all of them, so a number below what the
	// binary carries means the two disagree about the schema.
	SchemaVersion int

	// SizeBytes is what the database occupies. Zero when the engine cannot
	// say, which is not the same as an empty database.
	SizeBytes int64

	// Reachable reports whether the database answered.
	Reachable bool

	// Latency is how long the database took to answer.
	Latency time.Duration

	// Error is why the description is incomplete, and empty when it is not. A
	// database that never answered leaves Reachable false and every detail
	// unset; one that answered but would not describe itself stays reachable
	// and explains itself here.
	Error string

	// Connections is the pool the service holds against the database.
	Connections ConnectionStats
}

// ConnectionStats is the pool between the service and its database.
//
// A database the service reaches over a network is shared with itself: the
// panel, the delivery workers and the sweep all draw from one pool, and
// running out of it stalls them without failing anything. An engine that
// permits a single connection reports a Max of one and never contends.
type ConnectionStats struct {
	// Open is how many connections the pool holds, whether or not they are
	// running a statement.
	Open int

	// InUse is how many of them are running one.
	InUse int

	// Idle is how many are held open and unused.
	Idle int

	// Max is the ceiling the pool will not open past.
	Max int

	// WaitCount is how many callers have waited for a free connection since
	// the process started. It only ever grows, so unlike the counts above it
	// is evidence rather than a sample: a pool that has ever been exhausted
	// says so here long after the moment passed.
	WaitCount int64

	// WaitDuration is how long those callers waited in total.
	WaitDuration time.Duration
}
