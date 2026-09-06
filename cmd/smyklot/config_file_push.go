package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type configFilePush struct {
	Ref        string `json:"ref"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
}

func parseConfigFilePush(payload []byte) (configFilePush, error) {
	var push configFilePush
	if err := json.Unmarshal(payload, &push); err != nil {
		return push, fmt.Errorf("%w: %w", webhook.ErrMalformedPayload, err)
	}
	branch, isBranch := strings.CutPrefix(push.Ref, "refs/heads/")
	tag, isTag := strings.CutPrefix(push.Ref, "refs/tags/")
	if (!isBranch || branch == "") && (!isTag || tag == "") {
		return push, fmt.Errorf("%w: push has no branch or tag ref", webhook.ErrMalformedPayload)
	}
	return push, nil
}

func (push configFilePush) relevant() bool {
	return !push.Deleted && strings.HasPrefix(push.Ref, "refs/heads/") &&
		(push.Repository.DefaultBranch == "" || push.Ref == "refs/heads/"+push.Repository.DefaultBranch)
}

// A signed push is a wake-up, never a settings source. The reconciler reads the
// current default-branch tree, since GitHub may truncate the commit list and
// deliveries can arrive out of order. Branch pushes made by Smyklot are ignored
// until their proposal reaches the default branch.
func (s *server) notifyPushedConfigurationFile(ctx context.Context, delivery webhook.Delivery) error {
	push, err := parseConfigFilePush(delivery.Payload)
	if err != nil || !push.relevant() {
		return err
	}
	targetID := storage.InstallationID(delivery.Source.InstallationID)
	repositoryID := storage.RepositoryID(delivery.Source.Repository.ID)
	repository, err := s.store.GetRepository(ctx, targetID, repositoryID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil || !repository.Available {
		return err
	}
	if push.Repository.DefaultBranch == "" && push.Ref != "refs/heads/"+repository.DefaultBranch {
		return nil
	}
	now := time.Now().UTC()
	if err := s.store.NotifyConfigFileChange(ctx, targetID, repositoryID, now); err != nil {
		return err
	}
	if repository.Name == ".github" {
		if err := s.store.NotifyConfigFileChange(ctx, targetID, "", now); err != nil {
			return err
		}
	}
	s.WakeQueue(workqueue.LaneMaintenance)
	return nil
}
