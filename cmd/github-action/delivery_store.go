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
)

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
// recorded. A transient SQLite stall therefore delays redelivery instead of
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
	if s.store == nil {
		if s.deduper.Begin(j.key) {
			return storage.DeliveryClaimAccepted, nil
		}

		return storage.DeliveryClaimRetained, nil
	}
	repositoryID := repositoryStorageID(j.event.Repository.ID)
	fullName := j.event.Repository.FullName
	if fullName == "" {
		fullName = repoFullName(j.event.Repository.Owner.Login, j.event.Repository.Name)
	}

	result, err := s.store.ClaimDelivery(ctx, storage.DeliveryClaim{
		ClaimKey:           j.key,
		DeliveryID:         j.deliveryID,
		TargetID:           installationStorageID(j.event.Installation.ID),
		RepositoryID:       &repositoryID,
		RepositoryFullName: fullName,
		Event:              j.event.Action,
		ClaimedAt:          time.Now().UTC(),
	})
	if err != nil || result.Disposition != storage.DeliveryClaimAccepted {
		return result.Disposition, err
	}
	j.claimID = result.ID

	return storage.DeliveryClaimAccepted, nil
}

func (s *server) abandonDelivery(ctx context.Context, j job) error {
	if s.store == nil {
		s.deduper.Abandon(j.key)

		return nil
	}

	return s.store.AbandonDelivery(ctx, j.claimID)
}

func (s *server) completeDelivery(ctx context.Context, j job) error {
	if s.store == nil {
		return nil
	}
	if err := s.store.CompleteDelivery(ctx, j.claimID, time.Now().UTC()); err != nil {
		return err
	}
	s.announceDelivery(j)

	return nil
}

func (s *server) failDelivery(ctx context.Context, j job, cause error) error {
	if s.store == nil {
		s.deduper.Abandon(j.key)

		return nil
	}
	reason := s.redactor.Error(cause)
	if len(reason) > maxStoredFailureReason {
		reason = strings.TrimSpace(reason[:maxStoredFailureReason])
	}
	err := s.store.FailDelivery(ctx, storage.DeliveryFailureChange{
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

func (s *server) announceDelivery(j job) {
	if s.panel != nil {
		s.panel.Announce(
			installationStorageID(j.event.Installation.ID),
			repositoryStorageID(j.event.Repository.ID),
		)
	}
}
