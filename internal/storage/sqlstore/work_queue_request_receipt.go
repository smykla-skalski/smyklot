package sqlstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func validRecurringRequestKey(key string) bool {
	return key == "" || (len(key) <= 200 && strings.TrimSpace(key) == key)
}

// FindRecurringWorkRequest reads acceptance without changing work. The caller
// must authorize the actor and requested scope before exposing non-sync receipts.
// Explicit sync receipts also require current authority inside this read.
func (s *Store) FindRecurringWorkRequest(ctx context.Context, request workqueue.RecurringRequest) (workqueue.Item, error) {
	if request.RequestKey == "" || !validRecurringRequestKey(request.RequestKey) {
		return workqueue.Item{}, storage.ErrConflict
	}
	if request.Kind != workqueue.KindSyncScan {
		return recurringRequestReceipt(ctx, s.db, request)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.authorizeSyncCheckRequest(ctx, tx, request); err != nil {
		return workqueue.Item{}, err
	}
	return recurringRequestReceipt(ctx, tx, request)
}

func recurringRequestInput(request workqueue.RecurringRequest) string {
	// Encode each scope field separately; concatenated identifiers can collide.
	input, _ := json.Marshal(struct {
		Kind         workqueue.Kind
		TargetID     *string
		RepositoryID *string
		Title        string
		Reason       string
	}{request.Kind, request.TargetID, request.RepositoryID, request.Title, request.Reason})
	return string(input)
}

func recurringRequestReceipt(ctx context.Context, reader runner, request workqueue.RecurringRequest) (workqueue.Item, error) {
	var input, accepted string
	err := reader.QueryRowContext(ctx, `SELECT input_json, accepted_item FROM recurring_request_receipts WHERE actor_account_id = ? AND request_key = ?`, request.ActorID, request.RequestKey).Scan(&input, &accepted)
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("read recurring request receipt: %w", noRows(err))
	}
	if input != recurringRequestInput(request) {
		return workqueue.Item{}, storage.ErrConflict
	}
	var item workqueue.Item
	if err := json.Unmarshal([]byte(accepted), &item); err != nil {
		return workqueue.Item{}, fmt.Errorf("decode recurring request receipt: %w", err)
	}
	if item.ID == "" {
		return workqueue.Item{}, errors.New("recurring request receipt has no occurrence")
	}
	return item, nil
}

func insertRecurringRequestReceipt(ctx context.Context, tx *transaction, request workqueue.RecurringRequest, item workqueue.Item) error {
	if request.RequestKey == "" {
		return nil
	}
	accepted, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode recurring request receipt: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO recurring_request_receipts (actor_account_id, request_key, input_json, accepted_item, requested_at) VALUES (?, ?, ?, ?, ?)`, request.ActorID, request.RequestKey, recurringRequestInput(request), string(accepted), request.Now)
	if err != nil {
		return fmt.Errorf("record recurring request receipt: %w", err)
	}
	return nil
}
