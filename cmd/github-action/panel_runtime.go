package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/panelassets"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// candidateTTL is how long an organization roster is reused. Short enough that
// somebody who joined the organization this morning can be completed by the
// afternoon, long enough that a person typing a login costs no GitHub requests
// at all after the first.
const candidateTTL = 10 * time.Minute

type candidateRoster struct {
	accounts  []storage.Account
	expiresAt time.Time
}

func (s *server) initPanel() error {
	if s.cfg.panel == nil {
		return nil
	}

	assets, err := panelassets.Open()
	if err != nil {
		return err
	}
	apiURL := s.cfg.apiBaseURL
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	publicOrigin, err := url.Parse(s.cfg.panel.publicOrigin)
	if err != nil {
		return fmt.Errorf("parse panel public origin: %w", err)
	}
	panelServer, err := adminpanel.New(adminpanel.Config{
		BasePath:                 s.cfg.panel.basePath,
		PublicOrigin:             s.cfg.panel.publicOrigin,
		SuperRootID:              s.cfg.panel.superRootID,
		ClientID:                 s.cfg.panel.clientID,
		ClientSecret:             s.cfg.panel.clientSecret,
		AuthorizeURL:             s.cfg.panel.authorizeURL,
		TokenURL:                 s.cfg.panel.tokenURL,
		APIURL:                   apiURL,
		Version:                  version,
		ServiceHost:              publicOrigin.Host,
		ListenAddress:            s.cfg.listenAddress,
		AdminAddress:             s.cfg.adminAddress,
		WebhookPath:              s.cfg.webhookPath,
		LogLevel:                 s.cfg.logLevel,
		PollInterval:             s.cfg.pollInterval,
		PendingCIQuietPeriod:     s.cfg.pendingCIQuietPeriod,
		SessionTTL:               s.cfg.panel.sessionTTL,
		ProcessConfig:            s.cfg.botConfig,
		WebhookCredentialPresent: len(s.cfg.webhookSecret) > 0,
		AppCredentialPresent:     len(s.cfg.appPrivateKey) > 0,
		OAuthCredentialPresent:   s.cfg.panel.clientSecret != "",
		Assets:                   assets,
	}, adminpanel.Dependencies{
		Store: s.store, Catalog: s, Users: s, Runtime: s, Candidates: s,
		Gates: s,
		PendingCI: newPendingCIControl(
			s.store, s.pendingCICoordinator, s.pendingCI.Wake,
		),
	})
	if err != nil {
		return fmt.Errorf("initialize panel: %w", err)
	}

	s.panel = panelServer

	return nil
}

func (s *server) initStorage(ctx context.Context) error {
	store, err := open.Store(ctx, s.cfg.database)
	if err != nil {
		return fmt.Errorf("open service storage: %w", err)
	}
	if err := store.RecoverRunningDeliveries(ctx, time.Now().UTC()); err != nil {
		_ = store.Close()

		return fmt.Errorf("recover interrupted deliveries: %w", err)
	}

	s.store = store

	return nil
}

// ResolveUser resolves a login through one selected GitHub App installation.
func (s *server) ResolveUser(
	ctx context.Context,
	targetID, login string,
) (storage.Account, error) {
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return storage.Account{}, fmt.Errorf("read user lookup installation: %w", err)
	}
	if !target.Available {
		return storage.Account{}, fmt.Errorf("user lookup installation %q is unavailable", targetID)
	}
	installationID, err := strconv.ParseInt(target.InstallationID, 10, 64)
	if err != nil {
		return storage.Account{}, fmt.Errorf("parse user lookup installation id: %w", err)
	}
	if installationID <= 0 {
		return storage.Account{}, fmt.Errorf(
			"user lookup installation id %q must be positive",
			target.InstallationID,
		)
	}
	token, err := s.tokens.InstallationToken(installationID)
	if err != nil {
		return storage.Account{}, NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return storage.Account{}, NewGitHubError(ErrGitHubClient, err)
	}
	user, err := client.GetUser(ctx, login)
	if err != nil {
		return storage.Account{}, fmt.Errorf("resolve GitHub user: %w", err)
	}
	apiURL := s.cfg.apiBaseURL
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}

	return adminpanel.NewGitHubAccount(
		apiURL,
		user.ID,
		user.Login,
		user.Name,
		user.AvatarURL,
		time.Now().UTC(),
	)
}

// ResolveRootUser resolves a login through the first available installation.
// This keeps user lookup independent of regular-panel ownership while using
// the same least-privilege installation authentication as scoped invitations.
// ListTargetCandidates returns the people who could be given access to the
// installation: the members of the organization it belongs to.
//
// A personal installation has no roster, and returns none rather than an error -
// the panel offers no completion there and the field stays what it always was, a
// login typed in full.
//
// The roster is read whole and cached. GitHub pages it, so completing on every
// keystroke would be several requests per letter; membership changes on the
// scale of days, and the panel resolves whatever is typed on submit regardless,
// so a list a few minutes stale costs nothing and a wrong one is impossible.
func (s *server) ListTargetCandidates(
	ctx context.Context,
	targetID string,
) ([]storage.Account, error) {
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("read candidate installation: %w", err)
	}
	if target.Kind != storage.TargetOrganization || !target.Available {
		return nil, nil
	}
	if cached, ok := s.cachedCandidates(targetID); ok {
		return cached, nil
	}

	installationID, err := strconv.ParseInt(target.InstallationID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse candidate installation id: %w", err)
	}
	token, err := s.tokens.InstallationToken(installationID)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubClient, err)
	}
	members, err := client.ListOrganizationMembers(ctx, target.Account.Login)
	if err != nil {
		return nil, fmt.Errorf("list organization members: %w", err)
	}

	apiURL := s.cfg.apiBaseURL
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	now := time.Now().UTC()
	accounts := make([]storage.Account, 0, len(members))
	for _, member := range members {
		account, err := adminpanel.NewGitHubAccount(
			apiURL, member.ID, member.Login, member.Name, member.AvatarURL, now,
		)
		if err != nil {
			// One unusable record must not cost the whole roster.
			continue
		}
		accounts = append(accounts, account)
	}
	s.storeCandidates(targetID, accounts, now)

	return accounts, nil
}

func (s *server) cachedCandidates(targetID string) ([]storage.Account, bool) {
	s.candidatesMu.Lock()
	defer s.candidatesMu.Unlock()

	entry, ok := s.candidates[targetID]
	if !ok || time.Now().UTC().After(entry.expiresAt) {
		return nil, false
	}

	return entry.accounts, true
}

func (s *server) storeCandidates(targetID string, accounts []storage.Account, now time.Time) {
	s.candidatesMu.Lock()
	defer s.candidatesMu.Unlock()

	if s.candidates == nil {
		s.candidates = map[string]candidateRoster{}
	}
	s.candidates[targetID] = candidateRoster{
		accounts:  accounts,
		expiresAt: now.Add(candidateTTL),
	}
}

func (s *server) ResolveRootUser(ctx context.Context, login string) (storage.Account, error) {
	targets, err := s.store.ListRootTargets(ctx)
	if err != nil {
		return storage.Account{}, fmt.Errorf("list Root user lookup installations: %w", err)
	}
	for _, target := range targets {
		if target.Available {
			return s.ResolveUser(ctx, target.ID, login)
		}
	}

	return storage.Account{}, errors.New("no available installation can resolve the GitHub user")
}

// SyncCatalog refreshes the complete GitHub App installation catalog for an
// authenticated panel session. It commits only after every installation was
// read successfully, so a transient GitHub failure cannot hide valid targets.
func (s *server) SyncCatalog(ctx context.Context) ([]string, error) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	return s.syncPanelCatalogLocked(ctx)
}

func (s *server) syncPanelCatalogLocked(ctx context.Context) ([]string, error) {
	targetIDs, err := s.syncCatalog(ctx)
	if s.panel != nil {
		s.panel.AnnounceCatalog()
	}

	return targetIDs, err
}

func (s *server) syncCatalog(ctx context.Context) ([]string, error) {
	appToken, err := s.tokens.AppToken()
	if err != nil {
		return nil, NewGitHubError(ErrGitHubAppAuth, err)
	}
	appClient, err := github.NewAppClient(appToken, s.cfg.apiBaseURL)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubClient, err)
	}
	installations, err := appClient.ListInstallations(ctx)
	if err != nil {
		return nil, NewGitHubError(ErrListInstallations, err)
	}

	syncedAt := time.Now().UTC()
	snapshots := make([]storage.InstallationSnapshot, 0, len(installations))
	targetIDs := make([]string, 0, len(installations))
	for _, installation := range installations {
		snapshot, snapshotErr := s.loadInstallationSnapshot(ctx, installation, syncedAt)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		snapshots = append(snapshots, snapshot)
		targetIDs = append(targetIDs, snapshot.TargetID)
	}
	if err := s.reconcileCatalogSnapshots(ctx, snapshots); err != nil {
		return nil, fmt.Errorf("persist GitHub installation catalog: %w", err)
	}

	return targetIDs, nil
}

func (s *server) reconcileCatalogSnapshots(
	ctx context.Context,
	snapshots []storage.InstallationSnapshot,
) error {
	err := s.pendingCICoordinator.Exclusive(
		ctx, pendingCICatalogCoordinatorKey, func() error {
			repositoryIDs, idsErr := s.catalogRepositoryIDs(ctx, snapshots)
			if idsErr != nil {
				return idsErr
			}

			return exclusivePendingCIRepositories(
				ctx, s.pendingCICoordinator, repositoryIDs,
				func() error { return s.store.ReconcileCatalog(ctx, snapshots) },
			)
		},
	)
	if err == nil && s.pendingCI != nil {
		s.pendingCI.Wake()
	}

	return err
}

func (s *server) reconcileInstallationSnapshot(
	ctx context.Context,
	snapshot storage.InstallationSnapshot,
) error {
	return s.pendingCICoordinator.Exclusive(
		ctx, pendingCICatalogCoordinatorKey, func() error {
			repositoryIDs := snapshotRepositoryIDs([]storage.InstallationSnapshot{snapshot})
			current, err := s.store.ListRepositories(ctx, snapshot.TargetID)
			if err != nil {
				return fmt.Errorf(
					"list repositories for coordinated installation catalog: %w",
					err,
				)
			}
			for _, repository := range current {
				repositoryIDs = append(repositoryIDs, repository.ID)
			}

			return exclusivePendingCIRepositories(
				ctx, s.pendingCICoordinator, repositoryIDs,
				func() error { return s.store.ReconcileInstallation(ctx, snapshot) },
			)
		},
	)
}

func (s *server) catalogRepositoryIDs(
	ctx context.Context,
	snapshots []storage.InstallationSnapshot,
) ([]string, error) {
	targets, err := s.store.ListRootTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list targets for coordinated catalog: %w", err)
	}
	repositoryIDs := snapshotRepositoryIDs(snapshots)
	for _, target := range targets {
		repositories, listErr := s.store.ListRepositories(ctx, target.ID)
		if listErr != nil {
			return nil, fmt.Errorf("list repositories for coordinated catalog: %w", listErr)
		}
		for _, repository := range repositories {
			repositoryIDs = append(repositoryIDs, repository.ID)
		}
	}

	return repositoryIDs, nil
}

func snapshotRepositoryIDs(snapshots []storage.InstallationSnapshot) []string {
	var repositoryIDs []string
	for _, snapshot := range snapshots {
		for _, repository := range snapshot.Repositories {
			repositoryIDs = append(repositoryIDs, repository.ID)
		}
	}

	return repositoryIDs
}

func (s *server) loadInstallationSnapshot(
	ctx context.Context,
	installation github.Installation,
	syncedAt time.Time,
) (storage.InstallationSnapshot, error) {
	token, err := s.tokens.InstallationToken(installation.ID)
	if err != nil {
		return storage.InstallationSnapshot{}, NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return storage.InstallationSnapshot{}, NewGitHubError(ErrGitHubClient, err)
	}
	repositories, err := client.ListInstallationRepos(ctx)
	if err != nil {
		return storage.InstallationSnapshot{}, NewGitHubError(ErrListRepos, err)
	}

	return completeInstallationSnapshot(
		ctx, s.cfg.apiBaseURL, client, installation, repositories, syncedAt,
	)
}

func completeInstallationSnapshot(
	ctx context.Context,
	apiURL string,
	client *github.Client,
	installation github.Installation,
	repositories []github.Repository,
	syncedAt time.Time,
) (storage.InstallationSnapshot, error) {
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	snapshot, err := installationSnapshot(apiURL, installation, repositories, syncedAt)
	if err != nil {
		return storage.InstallationSnapshot{}, err
	}
	snapshot.Ownership = installationOwnership(ctx, apiURL, client, snapshot)

	return snapshot, nil
}

func installationOwnership(
	ctx context.Context,
	apiURL string,
	client *github.Client,
	snapshot storage.InstallationSnapshot,
) storage.OwnershipSnapshot {
	if snapshot.Kind == storage.TargetUser {
		return storage.OwnershipSnapshot{
			Source: storage.OwnershipSourcePersonal, Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{snapshot.Account}, SyncedAt: snapshot.SyncedAt,
		}
	}
	ownership := storage.OwnershipSnapshot{
		Source:   storage.OwnershipSourceOrganizationAdmin,
		Status:   storage.OwnershipStatusFresh,
		SyncedAt: snapshot.SyncedAt,
	}
	admins, err := client.ListOrganizationAdmins(ctx, snapshot.Account.Login)
	if err != nil {
		var apiErr *github.APIError
		detail := "organization owner synchronization failed"
		ownership.Status = storage.OwnershipStatusError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
			detail = "organization Members read permission requires installation approval"
			ownership.Status = storage.OwnershipStatusPermissionPending
		}
		ownership.Detail = &detail

		return ownership
	}
	for _, admin := range admins {
		account, accountErr := adminpanel.NewGitHubAccount(
			apiURL, admin.ID, admin.Login, admin.Name, admin.AvatarURL, snapshot.SyncedAt,
		)
		if accountErr != nil {
			detail := "organization owner synchronization returned an invalid identity"
			ownership.Status = storage.OwnershipStatusError
			ownership.Detail = &detail
			ownership.Owners = nil

			return ownership
		}
		ownership.Owners = append(ownership.Owners, account)
	}

	return ownership
}

func installationSnapshot(
	apiURL string,
	installation github.Installation,
	repositories []github.Repository,
	syncedAt time.Time,
) (storage.InstallationSnapshot, error) {
	if installation.ID <= 0 || installation.AccountID <= 0 || strings.TrimSpace(installation.Account) == "" {
		return storage.InstallationSnapshot{}, fmt.Errorf("GitHub installation identity is incomplete")
	}
	kind := storage.TargetKind(installation.AccountType)
	if kind != storage.TargetOrganization && kind != storage.TargetUser {
		return storage.InstallationSnapshot{}, fmt.Errorf(
			"unsupported GitHub installation account type %q", installation.AccountType)
	}
	provider := githubProvider(apiURL)
	subject := strconv.FormatInt(installation.AccountID, 10)
	accountKind := strings.ToLower(string(kind))
	accountID := provider + ":" + accountKind + ":" + subject
	avatarURL := stringPointerUnlessBlank(installation.AvatarURL)
	snapshot := storage.InstallationSnapshot{
		TargetID:       installationStorageID(installation.ID),
		InstallationID: strconv.FormatInt(installation.ID, 10),
		Kind:           kind,
		Account: storage.Account{
			ID:          accountID,
			Provider:    provider,
			SubjectID:   subject,
			Login:       installation.Account,
			DisplayName: installation.Account,
			AvatarURL:   avatarURL,
			UpdatedAt:   syncedAt,
		},
		Repositories: make([]storage.RepositorySnapshot, 0, len(repositories)),
		SyncedAt:     syncedAt,
		Permissions:  installation.Permissions,
	}
	for _, repository := range repositories {
		if repository.ID <= 0 || strings.TrimSpace(repository.Name) == "" {
			return storage.InstallationSnapshot{}, fmt.Errorf("GitHub repository identity is incomplete")
		}
		fullName := repository.FullName
		if fullName == "" {
			fullName = repoFullName(repository.Owner, repository.Name)
		}
		snapshot.Repositories = append(snapshot.Repositories, storage.RepositorySnapshot{
			ID:            repositoryStorageID(repository.ID),
			Name:          repository.Name,
			FullName:      fullName,
			Private:       repository.Private,
			DefaultBranch: repository.DefaultBranch,
		})
	}

	return snapshot, nil
}

func githubProvider(apiURL string) string {
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return "github:" + strings.TrimRight(apiURL, "/")
	}

	return "github:" + parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/")
}

func stringPointerUnlessBlank(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return &value
}

func installationStorageID(id int64) string {
	return "github:installation:" + strconv.FormatInt(id, 10)
}

func repositoryStorageID(id int64) string {
	return "github:repository:" + strconv.FormatInt(id, 10)
}
