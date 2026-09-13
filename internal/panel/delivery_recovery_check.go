package panel

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// DeliveryRecoveryChecker observes workload eligibility. Panel authorization and
// the storage recovery transaction remain separate, mandatory checks.
type DeliveryRecoveryChecker interface {
	CheckDeliveryRecovery(context.Context, storage.DeliveryRecoveryInput) (DeliveryRecoveryCheck, error)
}

type DeliveryRecoveryReason string

const (
	RecoveryReauthorizationUnavailable DeliveryRecoveryReason = "reauthorization_unavailable"
	RecoveryAvailable                  DeliveryRecoveryReason = "available"
	RecoveryPayloadInvalid             DeliveryRecoveryReason = "payload_invalid"
	RecoveryRepositoryUnavailable      DeliveryRecoveryReason = "repository_unavailable"
	RecoveryRepositoryDisabled         DeliveryRecoveryReason = "repository_disabled"
	RecoverySourceChanged              DeliveryRecoveryReason = "source_changed"
	RecoveryConfigurationInvalid       DeliveryRecoveryReason = "configuration_invalid"
	RecoveryPermissionChanged          DeliveryRecoveryReason = "permission_changed"
	RecoveryServiceDisabled            DeliveryRecoveryReason = "service_disabled"
	RecoveryNoAction                   DeliveryRecoveryReason = "no_action"
)

// DeliveryRecoveryCheck describes eligibility and the effect of a new run, not a promise of success.
type DeliveryRecoveryCheck struct {
	Reason DeliveryRecoveryReason
	Effect string
}
