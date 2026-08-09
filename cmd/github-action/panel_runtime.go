package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/panelassets"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func (s *server) initPanel(ctx context.Context) error {
	if s.cfg.panel == nil {
		return nil
	}

	assets, err := panelassets.Open()
	if err != nil {
		return err
	}
	store, err := sqlite.Open(ctx, s.cfg.panel.statePath)
	if err != nil {
		return fmt.Errorf("open panel storage: %w", err)
	}
	if err := store.RecoverRunningDeliveries(ctx, time.Now().UTC()); err != nil {
		_ = store.Close()

		return fmt.Errorf("recover interrupted panel deliveries: %w", err)
	}

	apiURL := s.cfg.apiBaseURL
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	publicOrigin, err := url.Parse(s.cfg.panel.publicOrigin)
	if err != nil {
		_ = store.Close()

		return fmt.Errorf("parse panel public origin: %w", err)
	}
	panelServer, err := adminpanel.New(adminpanel.Config{
		BasePath:      s.cfg.panel.basePath,
		PublicOrigin:  s.cfg.panel.publicOrigin,
		OwnerLogin:    s.cfg.panel.ownerLogin,
		ClientID:      s.cfg.panel.clientID,
		ClientSecret:  s.cfg.panel.clientSecret,
		AuthorizeURL:  s.cfg.panel.authorizeURL,
		TokenURL:      s.cfg.panel.tokenURL,
		APIURL:        apiURL,
		Version:       version,
		ServiceHost:   publicOrigin.Host,
		SessionTTL:    s.cfg.panel.sessionTTL,
		ProcessConfig: s.cfg.botConfig,
		Assets:        assets,
	}, adminpanel.Dependencies{Store: store, Catalog: s, Users: s})
	if err != nil {
		_ = store.Close()

		return fmt.Errorf("initialize panel: %w", err)
	}

	s.store = store
	s.panel = panelServer

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
	if err != nil {
		return nil, err
	}
	s.panel.AnnounceCatalog()

	return targetIDs, nil
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
	if err := s.store.ReconcileCatalog(ctx, snapshots); err != nil {
		return nil, fmt.Errorf("persist GitHub installation catalog: %w", err)
	}

	return targetIDs, nil
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

	return installationSnapshot(s.cfg.apiBaseURL, installation, repositories, syncedAt)
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
