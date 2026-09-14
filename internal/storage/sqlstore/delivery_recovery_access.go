package sqlstore

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Store) authorizeDeliveryRecovery(ctx context.Context, tx *transaction, request storage.DeliveryRecovery) error {
	return s.authorizeWorkspaceCommand(ctx, tx, workspaceCommandAuthority{ActorAccountID: request.ActorAccountID, SessionTokenHash: request.SessionTokenHash, TargetID: request.TargetID, ElevationID: request.ElevationID, RequestedAt: request.RequestedAt})
}
