package sqlstore

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func deliveryRecoveryAuthority(request storage.DeliveryRecovery, now func() time.Time) workspaceCommandAuthority {
	return workspaceCommandAuthority{ActorAccountID: request.ActorAccountID, SessionTokenHash: request.SessionTokenHash, TargetID: request.TargetID, ElevationID: request.ElevationID, Clock: now}
}
