package postgres_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
)

func overlapPendingCIWake(ctx context.Context, schema string, wake, lease func() error) (error, error) {
	raw := connect(ctx)
	defer func() { Expect(raw.Close()).To(Succeed()) }()
	guard, err := raw.Conn(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(guard.Close()).To(Succeed()) }()
	var guardPID int
	Expect(guard.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&guardPID)).To(Succeed())
	_, err = guard.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", schema)
	Expect(err).NotTo(HaveOccurred())
	released := false
	release := func() {
		if !released {
			_, releaseErr := guard.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock(hashtext($1))", schema)
			Expect(releaseErr).NotTo(HaveOccurred())
			released = true
		}
	}
	defer release()
	_, err = raw.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %[1]s.pause_pending_ci_wake() RETURNS trigger AS $$
BEGIN
    IF NEW.next_check_trigger = 'webhook' THEN
        PERFORM pg_advisory_xact_lock(hashtext('%[1]s'));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER pause_pending_ci_wake BEFORE UPDATE ON %[1]s.pending_ci_requests
FOR EACH ROW EXECUTE FUNCTION %[1]s.pause_pending_ci_wake();`, schema))
	Expect(err).NotTo(HaveOccurred())
	woken, leased := make(chan error, 1), make(chan error, 1)
	go func() { woken <- wake() }()
	var wakePID int
	Eventually(func() int {
		wakePID = blockedProcess(ctx, raw, guardPID)
		return wakePID
	}, 10*time.Second, 10*time.Millisecond).ShouldNot(BeZero())
	go func() { leased <- lease() }()
	Eventually(func() int { return blockedProcess(ctx, raw, wakePID) }, 10*time.Second, 10*time.Millisecond).
		ShouldNot(BeZero(), "the lease must overlap the source update")
	release()
	var wakeErr, leaseErr error
	Eventually(woken, 10*time.Second).Should(Receive(&wakeErr))
	Eventually(leased, 10*time.Second).Should(Receive(&leaseErr))
	return wakeErr, leaseErr
}
