package configsync

import (
	"errors"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// PrepareImport replaces only settings owned by this connection. Every Sync
// kind participates, including absent ones, so a concurrent creation cannot be
// missed and removing a kind from the file restores its absent state.
func (snapshot PanelSnapshot) PrepareImport(
	content []byte,
	source storage.ConfigFileImport,
	deploymentQuietPeriod time.Duration,
) (storage.SaveInstallationSettingsRequest, error) {
	document, err := DecodeDocument(content, snapshot.Scope())
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	if source.State.TargetID != snapshot.Target.ID || source.State.RepositoryID != snapshot.RepositoryID() ||
		source.State.OwnerRevision != snapshot.OwnerRevision() {
		return storage.SaveInstallationSettingsRequest{}, errors.New("configuration import snapshot does not match its connection")
	}
	settings := document.Panel.Settings
	quiet, err := durationValue(settings.QuietPeriod)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	interval, err := durationValue(settings.FileIndexInterval)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	if interval != nil && (*interval < 0 || *interval > storage.MaxPathIndexInterval) {
		return storage.SaveInstallationSettingsRequest{}, errors.New("file index interval is outside the supported range")
	}
	bypass, err := storageExceptions(settings.MergeExceptions, snapshot.Target.Kind)
	if err != nil {
		return storage.SaveInstallationSettingsRequest{}, err
	}
	request := storage.SaveInstallationSettingsRequest{
		TargetID: snapshot.Target.ID, ChangedAt: source.State.ChangedAt, ConfigFileImport: &source,
	}
	refs := storageRefs(settings.ProtectedRefs)
	if snapshot.Repository == nil {
		mode := storage.PendingCIMode(*settings.MergeMode)
		if err := storage.ValidateTargetPendingCISettings(mode, *refs, quiet); err != nil {
			return request, err
		}
		request.Target = &storage.InstallationTargetSettingsChange{
			ConfigFileSyncEnabled:    snapshot.Target.ConfigFileSyncEnabled,
			RepositoryDefaultEnabled: *settings.RepositoryDefaultEnabled,
			PendingCIModeDefault:     mode, PendingCIBranchPatternsDefault: *refs,
			PendingCIBypassPolicyDefault: bypass, PendingCIQuietPeriodOverride: quiet,
			PathIndexIntervalOverride: interval, ConfigPatch: document.Patch,
			ExpectedRevision: snapshot.Target.Revision, RetunePendingCIQuietPeriod: true,
			DeploymentPendingCIQuietPeriod: deploymentQuietPeriod,
		}
	} else {
		mode := storageMode(settings.MergeMode)
		if err := storage.ValidateRepositoryPendingCISettings(mode, refs, quiet); err != nil {
			return request, err
		}
		request.Repositories = []storage.InstallationRepositorySettingsChange{{
			RepositoryID: snapshot.Repository.ID, ConfigFileSyncEnabled: snapshot.Repository.ConfigFileSyncEnabled,
			EnabledOverride: settings.Enabled, PendingCIModeOverride: mode,
			PendingCIBranchPatternsOverride: refs, PendingCIBypassPolicyOverride: bypass,
			PendingCIQuietPeriodOverride: quiet, PathIndexIntervalOverride: interval,
			ConfigPatch: document.Patch, IgnoreRepositoryFile: snapshot.Repository.IgnoreRepositoryFile,
			ExpectedRevision: snapshot.Repository.Revision, RetunePendingCIQuietPeriod: true,
			DeploymentPendingCIQuietPeriod: deploymentQuietPeriod,
		}}
	}
	err = appendSyncImport(&request, snapshot, document.Panel)
	return request, err
}

func appendSyncImport(request *storage.SaveInstallationSettingsRequest, snapshot PanelSnapshot, section *config.PanelFileSection) error {
	revisions := snapshot.SyncRevisions()
	for _, kind := range orgsync.Kinds() {
		item, exists := section.Sync[string(kind)]
		document, err := preservedSyncDocument(snapshot, kind, item.Document, exists)
		if err != nil {
			return err
		}
		if snapshot.Repository == nil {
			enabled := item.Enabled != nil && *item.Enabled
			request.SyncConfigs = append(request.SyncConfigs, storage.InstallationSyncConfigChange{
				Kind: kind, Enabled: enabled, Document: document,
				ExpectedRevision: revisions[kind], Remove: !exists,
			})
		} else {
			request.SyncOverrides = append(request.SyncOverrides, storage.InstallationSyncOverrideChange{
				RepositoryID: snapshot.Repository.ID, Kind: kind, Enabled: item.Enabled,
				Document: document, ExpectedRevision: revisions[kind], Remove: !exists,
			})
		}
	}
	return nil
}

func storageMode(value *string) *storage.PendingCIMode {
	if value == nil {
		return nil
	}
	return new(storage.PendingCIMode(*value))
}

func storageRefs(value *config.PanelFileRefs) *storage.PendingCIBranchPatterns {
	if value == nil {
		return nil
	}
	return &storage.PendingCIBranchPatterns{Include: append([]string{}, value.Include...), Exclude: append([]string{}, value.Exclude...)}
}

func storageExceptions(value *config.PanelFileExceptions, kind storage.TargetKind) (*storage.PendingCIBypassPolicy, error) {
	if value == nil {
		return nil, nil
	}
	actors := make([]orgsync.RulesetBypassActor, 0, len(value.Actors))
	for _, actor := range value.Actors {
		var id int64
		if actor.ID != nil {
			id = *actor.ID
		}
		actors = append(actors, orgsync.RulesetBypassActor{ActorID: id, ActorType: actor.Type, Mode: actor.Mode})
	}
	policy := &storage.PendingCIBypassPolicy{Allow: value.Allow, Actors: actors}
	return policy, policy.ValidateForTarget(kind)
}
