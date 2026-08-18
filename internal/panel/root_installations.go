package panel

import (
	"errors"
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type rootTargetContext struct {
	Account     storage.Account
	Target      storage.Target
	Access      storage.TargetAccess
	SessionHash string
	Elevation   *storage.Elevation
}

func (s *Server) getRootTargetSettings(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, targetDTO(s.processConfig(), context.Target, context.Access))
}

func (s *Server) putRootTargetSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	context, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	var input targetSettingsRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.RepositoryDefaultEnabled == nil || input.ConfigPatch == nil || input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "target settings are incomplete")
		return
	}
	if err := validatePatch(*input.ConfigPatch); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	mode := context.Target.PendingCIModeDefault
	if input.PendingCIModeDefault != nil {
		mode = *input.PendingCIModeDefault
	}
	patterns := context.Target.PendingCIBranchPatternsDefault
	if input.PendingCIBranchPatternsDefault != nil {
		patterns = *input.PendingCIBranchPatternsDefault
	}
	quiet := context.Target.PendingCIQuietPeriodOverride
	if input.PendingCIQuietPeriodSeconds.Present {
		quiet = pendingCIQuietDuration(input.PendingCIQuietPeriodSeconds.Value)
	}
	if err := storage.ValidateTargetPendingCISettings(mode, patterns, quiet); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_pending_ci_settings", err.Error())
		return
	}
	updated, err := s.updateTargetSettings(r.Context(), storage.TargetSettingsChange{
		TargetID: context.Target.ID, ActorAccountID: context.Account.ID,
		ElevationID: elevationID(context.Elevation), SessionTokenHash: context.SessionHash,
		RepositoryDefaultEnabled:       *input.RepositoryDefaultEnabled,
		PendingCIModeDefault:           mode,
		PendingCIBranchPatternsDefault: patterns,
		PendingCIQuietPeriodOverride:   quiet,
		RetunePendingCIQuietPeriod:     input.PendingCIQuietPeriodSeconds.Present,
		DeploymentPendingCIQuietPeriod: s.cfg.PendingCIQuietPeriod,
		ConfigPatch:                    *input.ConfigPatch, ExpectedRevision: *input.ExpectedRevision,
		ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeRootWriteError(w, err)
		return
	}
	s.pendingCI.Wake()
	s.wakePendingCIGates()
	s.Announce(updated.ID, "")
	writeJSON(w, http.StatusOK, targetDTO(s.processConfig(), updated, context.Access))
}

func (s *Server) getRootRepositories(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	page, err := parseRepositoryPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_repository_query", err.Error())
		return
	}
	repositories, err := s.store.ListRepositoryPage(r.Context(), context.Target.ID, page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	context.Target.RepositoryDefaultEnabled = repositories.RepositoryDefaultEnabled
	writeJSON(w, http.StatusOK, repositoryPageDTO(context.Target, repositories))
}

func (s *Server) getRootRepository(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	repository, ok := s.repository(w, r, context.Target)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, repositoryDetailDTO(s.processConfig(), context.Target, repository))
}

func (s *Server) putRootRepositorySettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	context, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	repository, ok := s.repository(w, r, context.Target)
	if !ok {
		return
	}
	var input repositorySettingsRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.EnabledOverride.Present || input.ConfigPatch == nil ||
		input.IgnoreRepositoryFile == nil || input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "repository settings are incomplete")
		return
	}
	if err := validatePatch(*input.ConfigPatch); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	mode := repository.PendingCIModeOverride
	if input.PendingCIModeOverride.Present {
		mode = input.PendingCIModeOverride.Value
	}
	patterns := repository.PendingCIBranchPatternsOverride
	if input.PendingCIBranchPatternsOverride.Present {
		patterns = input.PendingCIBranchPatternsOverride.Value
	}
	quiet := repository.PendingCIQuietPeriodOverride
	if input.PendingCIQuietPeriodSeconds.Present {
		quiet = pendingCIQuietDuration(input.PendingCIQuietPeriodSeconds.Value)
	}
	if err := storage.ValidateRepositoryPendingCISettings(mode, patterns, quiet); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_pending_ci_settings", err.Error())
		return
	}
	updated, err := s.updateRepositorySettings(r.Context(), storage.RepositorySettingsChange{
		TargetID: context.Target.ID, RepositoryID: r.PathValue("repository"),
		ActorAccountID: context.Account.ID, ElevationID: elevationID(context.Elevation),
		SessionTokenHash: context.SessionHash, EnabledOverride: input.EnabledOverride.Value,
		PendingCIModeOverride:           mode,
		PendingCIBranchPatternsOverride: patterns,
		PendingCIQuietPeriodOverride:    quiet,
		RetunePendingCIQuietPeriod:      input.PendingCIQuietPeriodSeconds.Present,
		DeploymentPendingCIQuietPeriod:  s.cfg.PendingCIQuietPeriod,
		ConfigPatch:                     *input.ConfigPatch, IgnoreRepositoryFile: *input.IgnoreRepositoryFile,
		ExpectedRevision: *input.ExpectedRevision, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeRootWriteError(w, err)
		return
	}
	s.pendingCI.Wake()
	s.wakePendingCIGates()
	if !s.attachPendingCIGate(w, r, &updated) {
		return
	}
	s.Announce(context.Target.ID, updated.ID)
	writeJSON(w, http.StatusOK, repositoryDetailDTO(s.processConfig(), context.Target, updated))
}

func (s *Server) postRootRepositoryConfigMigrationReset(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	context, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	repository, ok := s.repository(w, r, context.Target)
	if !ok {
		return
	}
	s.resetConfigMigration(
		w, r, context.Target, repository,
		configMigrationActor{
			accountID: context.Account.ID, elevationID: elevationID(context.Elevation),
			sessionTokenHash: context.SessionHash,
		},
		s.writeRootWriteError,
	)
}

func (s *Server) requireRootTarget(
	w http.ResponseWriter,
	r *http.Request,
	write bool,
) (rootTargetContext, bool) {
	account, _, sessionHash, ok := s.requireRootSession(w, r)
	if !ok {
		return rootTargetContext{}, false
	}
	target, err := s.store.GetTarget(r.Context(), r.PathValue("target"))
	if err != nil {
		s.writeStorageError(w, err)
		return rootTargetContext{}, false
	}
	context := rootTargetContext{
		Account: account, Target: target, SessionHash: sessionHash,
		Access: rootReadAccess(),
	}
	if target.Available {
		access, accessErr := s.store.ResolveTargetAccess(
			r.Context(), account.ID, target.ID, s.now().UTC(),
		)
		if accessErr != nil {
			s.writeStorageError(w, accessErr)
			return rootTargetContext{}, false
		}
		if access.Role == storage.InstallationRoleOwner {
			context.Access = access
			return context, true
		}
	}
	if !write {
		if err := s.attachActiveElevation(r, &context); err != nil {
			s.writeInternal(w, err)
			return rootTargetContext{}, false
		}
		return context, true
	}
	if !target.Available {
		s.writeError(w, http.StatusConflict, "installation_unavailable", "the installation is unavailable")
		return rootTargetContext{}, false
	}
	elevation, err := s.store.GetElevation(r.Context(), sessionHash, target.ID, s.now().UTC())
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.writeError(w, http.StatusForbidden, "elevation_required", "start elevated access for this installation")
		} else {
			s.writeElevationError(w, err)
		}
		return rootTargetContext{}, false
	}
	if !target.Ownership.FreshAt(s.now().UTC()) {
		s.writeError(w, http.StatusConflict, "owner_snapshot_unavailable", "fresh Owners are required for elevated writes")
		return rootTargetContext{}, false
	}
	context.Elevation = &elevation
	context.Access.Source = storage.AccessSourceElevation
	context.Access.Capabilities.Write = true

	return context, true
}

func (s *Server) attachActiveElevation(r *http.Request, context *rootTargetContext) error {
	elevation, err := s.store.GetElevation(
		r.Context(), context.SessionHash, context.Target.ID, s.now().UTC(),
	)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrExpired) ||
			errors.Is(err, storage.ErrRevoked) {
			return nil
		}
		return err
	}
	context.Elevation = &elevation
	context.Access.Source = storage.AccessSourceElevation
	context.Access.Capabilities.Write = context.Target.Ownership.FreshAt(s.now().UTC())

	return nil
}

func rootReadAccess() storage.TargetAccess {
	return storage.TargetAccess{
		Role: storage.InstallationRoleNone, Source: storage.AccessSourceRoot, Root: true,
		Capabilities: storage.AccessCapabilities{Read: true},
	}
}

func elevationID(elevation *storage.Elevation) *string {
	if elevation == nil {
		return nil
	}

	return &elevation.ID
}

func (s *Server) writeRootWriteError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrExpired) || errors.Is(err, storage.ErrRevoked) {
		s.writeElevationError(w, err)
		return
	}
	if errors.Is(err, storage.ErrConflict) {
		s.writeError(w, http.StatusConflict, "owner_snapshot_unavailable", "fresh Owners are required for elevated writes")
		return
	}
	s.writeStorageError(w, err)
}
