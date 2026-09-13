package panel

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// This projection contains execution state, never a webhook payload or a grant
// to recover it. Read permissions were established before loading the queue item.
type deliveryOperationResponse struct {
	Retained bool                 `json:"retained"`
	Revision int64                `json:"revision"`
	Current  *deliveryRunResponse `json:"current"`
}

type deliveryRunResponse struct {
	ID               int64                     `json:"id"`
	Status           storage.DeliveryStatus    `json:"status"`
	PayloadAvailable bool                      `json:"payload_available"`
	Queue            *deliveryRunQueueResponse `json:"queue"`
}

type deliveryRunQueueResponse struct {
	ID         string          `json:"id"`
	State      workqueue.State `json:"state"`
	EligibleAt time.Time       `json:"eligible_at"`
}

func (s *Server) queueDeliveryOperation(r *http.Request, item workqueue.Item) (*deliveryOperationResponse, error) {
	if item.Kind != workqueue.KindWebhookDelivery || item.SourceKind != "delivery" || item.TargetID == nil {
		return nil, nil
	}
	runID, err := strconv.ParseInt(item.SourceID, 10, 64)
	if err != nil || runID <= 0 {
		return nil, storage.ErrConflict
	}
	operation, err := s.store.GetDeliveryOperation(r.Context(), *item.TargetID, runID)
	if errors.Is(err, storage.ErrNotFound) {
		return &deliveryOperationResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := &deliveryOperationResponse{Retained: true, Revision: operation.Revision}
	if operation.Current != nil {
		current := operation.Current
		result.Current = &deliveryRunResponse{ID: current.ID, Status: current.Status, PayloadAvailable: current.PayloadAvailable}
		if current.Queue != nil {
			result.Current.Queue = &deliveryRunQueueResponse{ID: current.Queue.ID, State: current.Queue.State, EligibleAt: current.Queue.EligibleAt}
		}
	}
	return result, nil
}
