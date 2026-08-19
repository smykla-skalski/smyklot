package storagetest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Seed writes one realistic row into every table the service uses.
//
// It goes through the port rather than issuing SQL, so what it produces is
// exactly what the application produces - a fixture written as INSERT
// statements would drift from the real write paths the first time one of them
// changed, and would then prove nothing about them.
//
// It returns errors rather than asserting, so a plain go test can use it as
// readily as a Ginkgo spec.
//
// SeededTables lists what it fills. A test that copies or exports a database
// can assert against that list instead of restating it.
func Seed(ctx context.Context, store storage.Store, now time.Time) error {
	seed := seeder{ctx: ctx, store: store, now: now}

	return seed.run()
}

// SeededTables names every table Seed leaves non-empty, so a test can assert a
// copy carried real rows rather than passing on a database of empty tables.
//
// panel_owner is deliberately absent: migration 003 moved its single row into
// panel_users, and nothing has written to it since. It survives only because
// dropping a table in SQLite means rebuilding it.
func SeededTables() []string {
	return []string{
		"accounts",
		"sessions",
		"panel_users",
		"targets",
		"target_ownership",
		"target_owners",
		"target_roles",
		"repositories",
		"pending_ci_repository_gates",
		"pending_ci_check_slots",
		"root_elevations",
		"audit_entries",
		"access_audit_entries",
		"app_audit_events",
		"security_notifications",
		"deliveries",
		"pending_ci_requests",
		"pending_ci_events",
		"pending_ci_intents",
		"pending_ci_source_revisions",
		"user_invitations",
		"runtime_settings",
		"user_preferences",
		"sync_configs",
		"sync_repository_overrides",
		"sync_repository_paths",
		"sync_repository_state",
		"sync_plans",
		"sync_plan_actions",
	}
}

// seeder carries the context, store and clock through the steps so each one
// reads as the write it performs rather than as five repeated arguments.
type seeder struct {
	ctx   context.Context
	store storage.Store
	now   time.Time

	root    storage.Account
	owner   storage.Account
	member  storage.Account
	invitee storage.Account

	target    storage.InstallationSnapshot
	session   storage.Session
	elevation storage.Elevation
}

func (s *seeder) run() error {
	steps := []struct {
		what string
		do   func() error
	}{
		{"catalog", s.seedCatalog},
		{"root session", s.seedRootSession},
		{"elevation", s.seedElevation},
		{"target access", s.seedTargetAccess},
		{"settings", s.seedSettings},
		{"invitation", s.seedInvitation},
		{"runtime settings", s.seedRuntimeSettings},
		{"preferences", s.seedPreferences},
		{"delivery", s.seedDelivery},
		{"pending CI", s.seedPendingCI},
		{"org sync", s.seedOrgSync},
	}

	for _, step := range steps {
		if err := step.do(); err != nil {
			return fmt.Errorf("seed %s: %w", step.what, err)
		}
	}

	return nil
}

// seedCatalog fills accounts, targets, target_ownership, target_owners and
// repositories from one installation reconcile, the way a sync does.
func (s *seeder) seedCatalog() error {
	s.owner = testAccount(s.now)
	s.target = testInstallation(s.owner, s.now, []storage.RepositorySnapshot{
		testRepository("repo-1", "smykla-skalski/smyklot", false),
		testRepository("repo-2", "smykla-skalski/klaudiush", true),
	})

	if err := s.store.ReconcileInstallation(s.ctx, s.target); err != nil {
		return err
	}

	return s.seedRepositoryFileState()
}

// seedRepositoryFileState fills the repository columns a reconcile leaves at
// their defaults.
//
// The copy is compared row by row, so a column left at its default on both
// sides proves nothing about whether it was carried across. Every one of these
// is written with a value nothing else would produce.
func (s *seeder) seedRepositoryFileState() error {
	problem := "line 7: command_aliases must be a mapping"
	if _, err := s.store.UpdateRepositoryFileState(s.ctx, storage.RepositoryFileState{
		TargetID:     s.target.TargetID,
		RepositoryID: "repo-1",
		Status:       storage.RepositoryFileInvalid,
		Error:        &problem,
		Path:         ".github/smyklot.yaml",
		Superseded:   []string{".smyklot/config.toml"},
		ObservedAt:   s.now,
	}); err != nil {
		return err
	}

	proposal := 41

	return s.store.SetRepositoryConfigMigration(s.ctx, storage.RepositoryConfigMigration{
		TargetID:     s.target.TargetID,
		RepositoryID: "repo-1",
		State:        storage.ConfigMigrationDeclined,
		PullRequest:  &proposal,
	})
}

// seedRootSession fills panel_users and sessions.
func (s *seeder) seedRootSession() error {
	s.root = derive(s.owner, "root", "Root Operator")
	if err := s.store.UpsertAccount(s.ctx, s.root); err != nil {
		return err
	}
	if err := s.store.ReconcileSuperRoot(s.ctx, s.root.ID, s.now); err != nil {
		return err
	}

	s.session = storage.Session{
		TokenHash: "seed-root-session",
		AccountID: s.root.ID,
		CreatedAt: s.now,
		ExpiresAt: s.now.Add(time.Hour),
	}

	return s.store.CreateSession(s.ctx, s.session, 4)
}

// seedElevation fills root_elevations, and app_audit_events with it.
func (s *seeder) seedElevation() error {
	reason := "seed a database worth copying"
	elevation, err := s.store.BeginElevation(s.ctx, storage.ElevationGrant{
		ID:               "seed-elevation",
		SessionTokenHash: s.session.TokenHash,
		RootAccountID:    s.root.ID,
		TargetID:         s.target.TargetID,
		Reason:           &reason,
		StartedAt:        s.now,
	})
	if err != nil {
		return err
	}
	s.elevation = elevation

	return nil
}

// seedTargetAccess fills target_roles, access_audit_entries and, because the
// write is elevated, security_notifications for every Owner.
func (s *seeder) seedTargetAccess() error {
	s.member = derive(s.owner, "member", "Installation Member")
	if err := s.store.UpsertAccount(s.ctx, s.member); err != nil {
		return err
	}
	if _, err := s.store.CreatePanelUser(s.ctx, storage.PanelUserCreate{
		AccountID:      s.member.ID,
		ActorAccountID: s.root.ID,
		ChangedAt:      s.now,
	}); err != nil {
		return err
	}

	role := storage.InstallationRoleEditor
	_, err := s.store.SetTargetAccess(s.ctx, storage.TargetAccessChange{
		TargetID:         s.target.TargetID,
		SubjectAccountID: s.member.ID,
		ActorAccountID:   s.root.ID,
		ElevationID:      &s.elevation.ID,
		SessionTokenHash: s.session.TokenHash,
		Role:             &role,
		ChangedAt:        s.now.Add(time.Minute),
	})

	return err
}

// seedSettings fills audit_entries through both settings paths, and leaves the
// repository holding a config patch so the jsonb column carries a document.
func (s *seeder) seedSettings() error {
	quiet := true
	if _, err := s.store.UpdateTargetSettings(s.ctx, storage.TargetSettingsChange{
		TargetID:                 s.target.TargetID,
		ActorAccountID:           s.root.ID,
		ElevationID:              &s.elevation.ID,
		SessionTokenHash:         s.session.TokenHash,
		RepositoryDefaultEnabled: true,
		ConfigPatch:              config.Patch{QuietSuccess: &quiet},
		ExpectedRevision:         1,
		ChangedAt:                s.now.Add(2 * time.Minute),
	}); err != nil {
		return err
	}

	enabled := true
	prefix := "/smyklot"
	_, err := s.store.UpdateRepositorySettings(s.ctx, storage.RepositorySettingsChange{
		TargetID:         s.target.TargetID,
		RepositoryID:     "repo-1",
		ActorAccountID:   s.root.ID,
		ElevationID:      &s.elevation.ID,
		SessionTokenHash: s.session.TokenHash,
		EnabledOverride:  &enabled,
		ConfigPatch:      config.Patch{CommandPrefix: &prefix},
		ExpectedRevision: 1,
		ChangedAt:        s.now.Add(3 * time.Minute),
	})

	return err
}

// seedInvitation fills user_invitations.
func (s *seeder) seedInvitation() error {
	s.invitee = derive(s.owner, "invitee", "Invited Member")
	if err := s.store.UpsertAccount(s.ctx, s.invitee); err != nil {
		return err
	}

	viewer := storage.InstallationRoleViewer
	_, err := s.store.CreateInvitation(s.ctx, storage.InvitationCreate{
		ID:               "seed-invitation",
		TokenHash:        "seed-invitation-token",
		AccountID:        s.invitee.ID,
		TargetID:         &s.target.TargetID,
		Role:             &viewer,
		ElevationID:      &s.elevation.ID,
		SessionTokenHash: s.session.TokenHash,
		ExpiresAt:        s.now.Add(24 * time.Hour),
		CreatedByAccount: s.root.ID,
		CreatedAt:        s.now.Add(4 * time.Minute),
	})

	return err
}

// seedRuntimeSettings fills runtime_settings, whose bot config is the second
// JSON document a copy has to carry intact.
func (s *seeder) seedRuntimeSettings() error {
	botConfig := config.Default()
	botConfig.QuietSuccess = true
	logLevel := "debug"
	pollInterval := 3 * time.Minute
	pendingCIQuietPeriod := 45 * time.Second
	sessionTTL := 12 * time.Hour

	_, err := s.store.UpdateRuntimeSettings(s.ctx, storage.RuntimeSettingsChange{
		BotConfig:                     botConfig,
		LogLevel:                      &logLevel,
		PollInterval:                  &pollInterval,
		PendingCIQuietPeriod:          &pendingCIQuietPeriod,
		SessionTTL:                    &sessionTTL,
		EffectivePollInterval:         pollInterval,
		EffectivePendingCIQuietPeriod: pendingCIQuietPeriod,
		EffectiveSessionTTL:           sessionTTL,
		ExpectedRevision:              0,
		ActorAccountID:                s.root.ID,
		ChangedAt:                     s.now.Add(5 * time.Minute),
	})

	return err
}

// seedPreferences fills user_preferences.
func (s *seeder) seedPreferences() error {
	_, err := s.store.ApplyPreferences(s.ctx, storage.PreferenceChange{
		AccountID: s.root.ID,
		Changes: map[string]json.RawMessage{
			"theme":   json.RawMessage(`"dark"`),
			"sidebar": json.RawMessage(`"collapsed"`),
		},
		ChangedAt: s.now.Add(6 * time.Minute),
	})

	return err
}

// seedDelivery fills deliveries with one finished claim and one that failed
// retryably, so a copy carries both the success and the failure shape.
func (s *seeder) seedDelivery() error {
	done, err := s.store.ClaimDelivery(s.ctx, storage.DeliveryClaim{
		ClaimKey:           "seed:delivery:done",
		DeliveryID:         "seed-delivery-done",
		TargetID:           s.target.TargetID,
		RepositoryFullName: "smykla-skalski/smyklot",
		Event:              "issue_comment",
		Payload:            []byte(`{"action":"created"}`),
		ClaimedAt:          s.now.Add(7 * time.Minute),
	})
	if err != nil {
		return err
	}
	if err := s.store.CompleteDelivery(s.ctx, done.ID, s.now.Add(8*time.Minute)); err != nil {
		return err
	}

	failed, err := s.store.ClaimDelivery(s.ctx, storage.DeliveryClaim{
		ClaimKey:           "seed:delivery:failed",
		DeliveryID:         "seed-delivery-failed",
		TargetID:           s.target.TargetID,
		RepositoryFullName: "smykla-skalski/klaudiush",
		Event:              "issue_comment",
		Payload:            []byte(`{"action":"edited"}`),
		ClaimedAt:          s.now.Add(9 * time.Minute),
	})
	if err != nil {
		return err
	}

	return s.store.FailDelivery(s.ctx, storage.DeliveryFailureChange{
		ClaimID:   failed.ID,
		Stage:     "github",
		Reason:    "provider timeout",
		Retryable: true,
		FailedAt:  s.now.Add(10 * time.Minute),
	})
}

// seedPendingCI fills the check slot and pending request tables with one
// terminal exact-head request.
func (s *seeder) seedPendingCI() error {
	requestedAt := s.now.Add(11 * time.Minute)
	checkSlot, err := s.store.EnsureCheckSlot(s.ctx, pendingci.EnsureCheckSlotRequest{
		TargetID: s.target.TargetID, InstallationID: 77,
		RepositoryID: "repo-1", RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest: 198, HeadSHA: "seed-head", AppID: 17,
		Name:          storage.PendingCICheckName,
		ExternalID:    "smyklot:merge-after-ci:repo-1:seed-head",
		DesiredStatus: "in_progress", DesiredTitle: "Merge authorized",
		DesiredSummary: "Waiting for CI", DesiredDigest: "seed-check",
		ChangedAt: requestedAt,
	})
	if err != nil {
		return err
	}
	claim, err := s.store.ClaimSourceRevision(s.ctx, pendingci.SourceRevisionRequest{
		RepositoryID: "repo-1", PullRequest: 198, CommentID: 101,
		Revision: requestedAt.Format(time.RFC3339Nano), Sequence: 1,
		SourceOrder: 1,
		EventKey:    "seed:pending-ci:source", ObservedAt: requestedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Accepted {
		return fmt.Errorf("claim seeded pending CI source revision")
	}

	armed, err := s.store.Arm(s.ctx, pendingci.ArmRequest{
		TargetID: s.target.TargetID, InstallationID: 77,
		RepositoryID: "repo-1", RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest: 198, HeadSHA: "seed-head", BaseBranch: "main",
		MergeMethod: pendingci.MergeMethodSquash, RequiredChecksOnly: true,
		Requester: "seed-owner", SourceCommentID: 101,
		SourceRevision: requestedAt.Format(time.RFC3339Nano),
		SourceSequence: 1, SourceOrder: claim.SourceOrder,
		ArtifactKind: pendingci.ArtifactCheck, CheckSlotID: &checkSlot.ID,
		RequestedAt: requestedAt,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Finish(s.ctx, pendingci.FinishRequest{
		ID: armed.Request.ID, ExpectedRevision: armed.Request.Revision,
		Lifecycle: pendingci.LifecycleMerged, Reason: "seeded terminal request",
		FinishedAt: requestedAt.Add(time.Minute),
	})

	return err
}

// seedOrgSync fills every sync table with a row nothing else would produce.
//
// A finished plan and a live one, because the two exercise different columns:
// the finished one carries approval, lease and outcome timestamps and an
// applied digest, and the live one holds the installation's single live slot.
// A copy compared row by row proves nothing about a column left at its default
// on both sides.
func (s *seeder) seedOrgSync() error {
	document := []byte(`{"labels":[{"name":"seeded","color":"d73a4a"}]}`)

	config, err := s.store.SetSyncConfig(s.ctx, orgsync.ConfigChange{
		TargetID: s.target.TargetID, Kind: orgsync.KindLabels, Enabled: true,
		Document: document, ActorID: s.owner.ID, Now: s.now,
	})
	if err != nil {
		return err
	}

	disabled := false
	if _, err := s.store.SetSyncRepositoryOverride(
		s.ctx, orgsync.RepositoryOverrideChange{
			RepositoryID: "repo-2", Kind: orgsync.KindLabels, Enabled: &disabled,
			ActorID: s.owner.ID, Now: s.now,
		}); err != nil {
		return err
	}

	// A second override, carrying a document rather than an answer about
	// whether the kind runs. Both halves of the row are filled, so a copy
	// between engines is proven on one that has something in every column
	// rather than on one that is empty on both sides.
	if _, err := s.store.SetSyncRepositoryOverride(
		s.ctx, orgsync.RepositoryOverrideChange{
			RepositoryID: "repo-1", Kind: orgsync.KindFiles,
			Document: []byte(`{"excludes":["renovate.json"]}`),
			ActorID:  s.owner.ID, Now: s.now,
		}); err != nil {
		return err
	}

	// What the panel's path finder offers. Two repositories rather than one, so
	// a copy is proven on a list that has to be aggregated rather than on a
	// single row that reads the same either way.
	if err := s.store.SetSyncRepositoryPaths(s.ctx, orgsync.RepositoryPaths{
		RepositoryID: "repo-1", TargetID: s.target.TargetID,
		Paths:      []string{"README.md", ".github/workflows/test.yaml"},
		ObservedAt: s.now,
		HeadSHA:    "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
	}); err != nil {
		return err
	}
	if err := s.store.SetSyncRepositoryPaths(s.ctx, orgsync.RepositoryPaths{
		RepositoryID: "repo-2", TargetID: s.target.TargetID,
		Paths:      []string{"README.md"},
		ObservedAt: s.now,
		HeadSHA:    "8765432109f8e7d6c5b4a3928170695f4e3d2c1b",
	}); err != nil {
		return err
	}

	if err := s.seedFinishedSyncPlan(config.Digest); err != nil {
		return err
	}

	// The live one is created last so it is the one holding the slot.
	_, err = s.store.CreateSyncPlan(s.ctx, orgsync.PlanCreate{
		ID: "sync-plan-live", TargetID: s.target.TargetID,
		Trigger: orgsync.TriggerReconcile, ActorID: s.owner.ID,
		Digest: config.Digest, Now: s.now.Add(time.Minute),
		ExpiresAt: s.now.Add(time.Hour),
		Actions: []orgsync.Action{{
			RepositoryID: "repo-1", Kind: orgsync.KindLabels,
			Operation: orgsync.OperationUpdate, Subject: "seeded",
			Before: "seeded #000000", After: "seeded #d73a4a",
			Payload: []byte(`{"name":"seeded","color":"d73a4a","description":"seeded"}`),
		}},
	})

	return err
}

// seedFinishedSyncPlan leaves a plan that has been through every state, so the
// approval, lease and outcome columns carry values rather than nulls.
func (s *seeder) seedFinishedSyncPlan(digest string) error {
	if _, err := s.store.CreateSyncPlan(s.ctx, orgsync.PlanCreate{
		ID: "sync-plan-done", TargetID: s.target.TargetID,
		Trigger: orgsync.TriggerManual, ActorID: s.owner.ID, Digest: digest,
		Now: s.now, ExpiresAt: s.now.Add(time.Hour),
		Actions: []orgsync.Action{{
			RepositoryID: "repo-1", Kind: orgsync.KindLabels,
			Operation: orgsync.OperationCreate, Subject: "seeded",
			After:   "seeded #d73a4a",
			Payload: []byte(`{"name":"seeded","color":"d73a4a","description":"seeded"}`),
		}},
	}); err != nil {
		return err
	}

	if _, err := s.store.ApproveSyncPlan(s.ctx, orgsync.PlanApproval{
		TargetID: s.target.TargetID, PlanID: "sync-plan-done", Digest: digest,
		ActorID: s.owner.ID, Now: s.now,
	}); err != nil {
		return err
	}

	lease, err := s.store.LeaseSyncPlan(s.ctx, s.now, s.now.Add(time.Minute))
	if err != nil {
		return err
	}
	for _, action := range lease.Actions {
		if err := s.store.RecordSyncActionOutcome(s.ctx, orgsync.ActionOutcome{
			ActionID: action.ID, State: orgsync.ActionApplied,
		}); err != nil {
			return err
		}
	}

	if err := s.store.FinishSyncPlan(s.ctx, orgsync.PlanOutcome{
		PlanID: "sync-plan-done", State: orgsync.PlanApplied,
		Now: s.now.Add(30 * time.Second),
		Applied: []orgsync.RepositoryState{{
			RepositoryID: "repo-1", Kind: orgsync.KindLabels,
			AppliedDigest: digest, AppliedAt: s.now.Add(30 * time.Second),
		}},
	}); err != nil {
		return err
	}

	// And one repository refused, so a copy is proven on a state row that says
	// why rather than only on one that says a digest. A column every seeded row
	// leaves at its default is a column a copy cannot be shown to carry.
	return s.store.RecordSyncRepositoryState(s.ctx, []orgsync.RepositoryState{{
		RepositoryID: "repo-2", Kind: orgsync.KindFiles,
		AppliedAt: s.now.Add(30 * time.Second),
		Problem:   "these files cannot be composed: docs is not a directory here",
	}})
}

// derive makes a second account from the first, so the seeded people differ in
// the fields that matter and agree on the rest.
func derive(from storage.Account, subject, displayName string) storage.Account {
	account := from
	account.ID = "github:" + subject
	account.SubjectID = subject
	account.Login = subject
	account.DisplayName = displayName

	return account
}
