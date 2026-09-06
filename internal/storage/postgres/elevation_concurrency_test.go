package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
)

// Pause termination after it locks the grant row, then let the real Store write
// acquire its target lock and wait on that grant. Releasing the pause forces the
// revocation audit's foreign-key check to overlap those locks deterministically.
func overlapElevationEnd(ctx context.Context, schema string, end, write func() error) (error, error) {
	raw := connect(ctx)
	defer func() { Expect(raw.Close()).To(Succeed()) }()
	guard, err := raw.Conn(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(guard.Close()).To(Succeed()) }()
	var guardPID int
	Expect(guard.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&guardPID)).To(Succeed())
	_, err = guard.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", schema)
	Expect(err).NotTo(HaveOccurred())
	unlocked := false
	release := func() {
		if !unlocked {
			_, releaseErr := guard.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock(hashtext($1))", schema)
			Expect(releaseErr).NotTo(HaveOccurred())
			unlocked = true
		}
	}
	defer release()
	_, err = raw.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %[1]s.pause_elevation_end() RETURNS trigger AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('%[1]s'));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER pause_elevation_end BEFORE UPDATE ON %[1]s.root_elevations
FOR EACH ROW EXECUTE FUNCTION %[1]s.pause_elevation_end();`, schema))
	Expect(err).NotTo(HaveOccurred())
	ended := make(chan error, 1)
	written := make(chan error, 1)
	go func() { ended <- end() }()
	var endPID int
	Eventually(func() bool {
		endPID = blockedProcess(ctx, raw, guardPID)
		return endPID != 0
	}, 10*time.Second, 10*time.Millisecond).Should(BeTrue(), "termination must hold the grant before the write starts")
	go func() { written <- write() }()
	Eventually(func() int { return blockedProcess(ctx, raw, endPID) }, 10*time.Second, 10*time.Millisecond).
		ShouldNot(BeZero(), "the protected write must wait on the ending grant")
	release()
	var endErr, writeErr error
	Eventually(ended, 10*time.Second).Should(Receive(&endErr))
	Eventually(written, 10*time.Second).Should(Receive(&writeErr))
	return endErr, writeErr
}

func blockedProcess(ctx context.Context, raw *sql.DB, blocker int) int {
	var pid int
	err := raw.QueryRowContext(ctx, `SELECT pid FROM pg_stat_activity
WHERE $1 = ANY(pg_blocking_pids(pid)) AND pid <> $1 LIMIT 1`, blocker).Scan(&pid)
	if err == sql.ErrNoRows {
		return 0
	}
	Expect(err).NotTo(HaveOccurred())
	return pid
}
