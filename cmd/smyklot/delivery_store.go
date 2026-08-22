package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

const (
	maxStoredFailureReason            = 2048
	deliveryFinalizationRetryInterval = time.Second
	deliveryRetryBaseDelay            = 2 * time.Second
	deliveryRetryMaxDelay             = 5 * time.Minute
	maxDeliveryAttempts               = 8
)

// deliveryState is the command worker's persistence boundary. Keeping this
// separate from storage.Store prevents delivery execution from acquiring panel
// or catalog responsibilities.
type deliveryState interface {
	ClaimDelivery(context.Context, storage.DeliveryClaim) (storage.DeliveryClaimResult, error)
	LeaseDelivery(context.Context, time.Time, time.Time) (storage.DeliveryLeaseResult, error)
	RetryDelivery(context.Context, storage.DeliveryRetryChange) error
	CompleteDelivery(context.Context, int64, time.Time) error
	FailDelivery(context.Context, storage.DeliveryFailureChange) error
}

type deliveryFinalizer func(context.Context) error

func deliveryFinalizationContext(executionContext context.Context) (
	context.Context,
	context.CancelFunc,
) {
	return context.WithTimeout(
		context.WithoutCancel(executionContext),
		deliveryFinalizationTimeout,
	)
}

// retryDeliveryFinalization keeps a durable claim owned until its outcome is
// recorded. A transient database stall therefore delays redelivery instead of
// stranding the claim for the rest of the process lifetime. Close cancels and
// joins these retries before closing the database; startup recovery handles a
// claim left by shutdown.
func (s *server) retryDeliveryFinalization(
	j job,
	outcome string,
	finalize deliveryFinalizer,
) {
	s.deliveryRetryMu.Lock()
	if s.deliveryRetryClosed {
		s.deliveryRetryMu.Unlock()

		return
	}
	s.deliveryRetries.Add(1)
	retryCtx := s.deliveryRetryCtx
	s.deliveryRetryMu.Unlock()

	go func() {
		defer s.deliveryRetries.Done()

		attempts := 0
		for {
			select {
			case <-retryCtx.Done():
				return
			default:
			}

			attempts++
			attemptCtx, cancel := context.WithTimeout(
				logging.Into(retryCtx, j.logger),
				deliveryFinalizationTimeout,
			)
			err := finalize(attemptCtx)
			cancel()
			if err == nil {
				logging.From(attemptCtx).Info(
					"delivery outcome persisted after retry",
					"outcome", outcome,
					"attempts", attempts,
				)

				return
			}

			timer := time.NewTimer(deliveryFinalizationRetryInterval)
			select {
			case <-retryCtx.Done():
				if !timer.Stop() {
					<-timer.C
				}

				return
			case <-timer.C:
			}
		}
	}()
}

func (s *server) stopDeliveryFinalizationRetries() {
	s.deliveryRetryMu.Lock()
	if !s.deliveryRetryClosed {
		s.deliveryRetryClosed = true
		if s.cancelDeliveryRetry != nil {
			s.cancelDeliveryRetry()
		}
	}
	s.deliveryRetryMu.Unlock()
	s.deliveryRetries.Wait()
}

func (s *server) beginDelivery(
	ctx context.Context,
	j *job,
) (storage.DeliveryClaimDisposition, error) {
	repositoryID := repositoryStorageID(j.metadata.RepositoryID)

	result, err := s.deliveryStore.ClaimDelivery(ctx, storage.DeliveryClaim{
		ClaimKey:           j.key,
		DeliveryID:         j.deliveryID,
		TargetID:           installationStorageID(j.metadata.InstallationID),
		RepositoryID:       &repositoryID,
		RepositoryFullName: j.metadata.RepositoryFullName,
		Event:              j.eventName,
		Payload:            j.payload,
		ClaimedAt:          time.Now().UTC(),
	})
	if err != nil || result.Disposition != storage.DeliveryClaimAccepted {
		return result.Disposition, err
	}
	j.claimID = result.ID

	return storage.DeliveryClaimAccepted, nil
}

// deliveryClaimKey uses GitHub's stable delivery GUID for real webhook
// identity. GitHub documents that redelivery retains this GUID. ContentKey is
// only a compatibility fallback for synthetic callers without the header.
func deliveryClaimKey(eventName, deliveryID, contentKey string) string {
	if deliveryID == "" || deliveryID == "unknown" {
		return contentKey
	}

	return "github-delivery:" + eventName + ":" + deliveryID
}

func (s *server) completeDelivery(ctx context.Context, j job) error {
	if err := s.deliveryStore.CompleteDelivery(ctx, j.claimID, time.Now().UTC()); err != nil {
		return err
	}
	s.announceDelivery(j)

	return nil
}

func (s *server) failDelivery(ctx context.Context, j job, cause error) error {
	reason := s.redactor.Error(cause)
	if len(reason) > maxStoredFailureReason {
		reason = strings.TrimSpace(reason[:maxStoredFailureReason])
	}
	err := s.deliveryStore.FailDelivery(ctx, storage.DeliveryFailureChange{
		ClaimID:   j.claimID,
		Stage:     "execute",
		Reason:    reason,
		Retryable: !errors.Is(cause, ErrRepoConfigInvalid),
		FailedAt:  time.Now().UTC(),
	})
	if err == nil {
		s.announceDelivery(j)
	}

	return err
}

func (s *server) retryDelivery(ctx context.Context, j job, cause error) error {
	reason := s.redactor.Error(cause)
	if len(reason) > maxStoredFailureReason {
		reason = strings.TrimSpace(reason[:maxStoredFailureReason])
	}
	err := s.deliveryStore.RetryDelivery(ctx, storage.DeliveryRetryChange{
		ClaimID: j.claimID,
		Stage:   "execute",
		Reason:  reason,
		RetryAt: time.Now().UTC().Add(deliveryRetryDelay(j.attempt)),
	})
	if err == nil {
		s.deliveries.Wake()
	}

	return err
}

func deliveryRetryDelay(attempt int) time.Duration {
	delay := deliveryRetryBaseDelay
	for index := 1; index < attempt && delay < deliveryRetryMaxDelay; index++ {
		delay *= 2
	}
	if delay > deliveryRetryMaxDelay {
		return deliveryRetryMaxDelay
	}

	return delay
}

func retryableDelivery(cause error, attempt int) bool {
	if attempt >= maxDeliveryAttempts || errors.Is(cause, ErrRepoConfigInvalid) {
		return false
	}

	var classified interface{ Retryable() bool }
	if errors.As(cause, &classified) {
		return classified.Retryable()
	}

	return true
}

func (s *server) announceDelivery(j job) {
	if s.panel != nil {
		s.panel.Announce(
			installationStorageID(j.metadata.InstallationID),
			repositoryStorageID(j.metadata.RepositoryID),
		)
	}
}
