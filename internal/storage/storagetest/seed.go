package storagetest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
		"root_elevations",
		"audit_entries",
		"access_audit_entries",
		"app_audit_events",
		"security_notifications",
		"deliveries",
		"user_invitations",
		"runtime_settings",
		"user_preferences",
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

	return s.store.ReconcileInstallation(s.ctx, s.target)
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
	sessionTTL := 12 * time.Hour

	_, err := s.store.UpdateRuntimeSettings(s.ctx, storage.RuntimeSettingsChange{
		BotConfig:             botConfig,
		LogLevel:              &logLevel,
		PollInterval:          &pollInterval,
		SessionTTL:            &sessionTTL,
		EffectivePollInterval: pollInterval,
		EffectiveSessionTTL:   sessionTTL,
		ExpectedRevision:      0,
		ActorAccountID:        s.root.ID,
		ChangedAt:             s.now.Add(5 * time.Minute),
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
