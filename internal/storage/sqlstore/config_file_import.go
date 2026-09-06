package sqlstore

import (
	"context"
	"encoding/hex"
	"errors"
	"path"
	"strings"
	"unicode"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func validateConfigFileImportRequest(request storage.SaveInstallationSettingsRequest) error {
	source := request.ConfigFileImport
	if err := source.State.Validate(); err != nil {
		return err
	}
	if source.State.TargetID != request.TargetID || !source.State.ChangedAt.Equal(request.ChangedAt) ||
		request.ElevationID != nil || request.SessionTokenHash != "" {
		return errors.New("configuration file import identity does not match the settings transaction")
	}
	if source.Path == "" || len(source.Path) > 4096 || path.IsAbs(source.Path) ||
		path.Clean(source.Path) != source.Path || source.Path == "." ||
		strings.HasPrefix(source.Path, "../") || strings.ContainsFunc(source.Path, unicode.IsControl) {
		return errors.New("configuration file import needs a relative file path")
	}
	if _, err := hex.DecodeString(source.HeadSHA); err != nil ||
		(len(source.HeadSHA) != 40 && len(source.HeadSHA) != 64) {
		return errors.New("configuration file import needs an immutable commit id")
	}
	if source.State.RepositoryID == "" {
		return validateWorkspaceFileImport(request)
	}
	return validateRepositoryFileImport(request)
}

func validateWorkspaceFileImport(request storage.SaveInstallationSettingsRequest) error {
	source := request.ConfigFileImport
	if request.Target == nil || !request.Target.ConfigFileSyncEnabled ||
		request.Target.ExpectedRevision != source.State.OwnerRevision ||
		len(request.Repositories) != 0 || len(request.SyncOverrides) != 0 ||
		len(request.SyncConfigs) != len(orgsync.Kinds()) {
		return errors.New("workspace configuration file import must contain only its complete workspace settings")
	}
	for _, item := range request.SyncConfigs {
		if item.ExpectedRevision != source.State.SyncRevisions[item.Kind] {
			return errors.New("configuration file import sync revision does not match its observation")
		}
	}
	return nil
}

func validateRepositoryFileImport(request storage.SaveInstallationSettingsRequest) error {
	source := request.ConfigFileImport
	if request.Target != nil || len(request.SyncConfigs) != 0 || len(request.Repositories) != 1 ||
		len(request.SyncOverrides) != len(orgsync.Kinds()) {
		return errors.New("repository configuration file import must contain only its complete repository settings")
	}
	repository := request.Repositories[0]
	if repository.RepositoryID != source.State.RepositoryID || !repository.ConfigFileSyncEnabled ||
		repository.ExpectedRevision != source.State.OwnerRevision {
		return errors.New("configuration file import repository does not match its observation")
	}
	for _, item := range request.SyncOverrides {
		if item.RepositoryID != source.State.RepositoryID ||
			item.ExpectedRevision != source.State.SyncRevisions[item.Kind] {
			return errors.New("configuration file import sync scope does not match its observation")
		}
	}
	return nil
}

func prepareConfigFileImport(ctx context.Context, tx *transaction, request storage.SaveInstallationSettingsRequest) error {
	if request.ConfigFileImport == nil {
		return nil
	}
	if err := verifyConfigFileChange(ctx, tx, request.ConfigFileImport.State); err != nil {
		return err
	}
	target, err := getTarget(ctx, tx, request.TargetID)
	if err != nil {
		return err
	}
	if request.Target != nil {
		if err := request.Target.PendingCIBypassPolicyDefault.ValidateForTarget(target.Kind); err != nil {
			return err
		}
	}
	if len(request.Repositories) == 1 {
		if err := request.Repositories[0].PendingCIBypassPolicyOverride.ValidateForTarget(target.Kind); err != nil {
			return err
		}
		current, err := getRepository(ctx, tx, request.TargetID, request.Repositories[0].RepositoryID)
		if err != nil {
			return err
		}
		if current.IgnoreRepositoryFile != request.Repositories[0].IgnoreRepositoryFile {
			return errors.New("configuration file import cannot change its own file bypass")
		}
	}
	return upsertCatalogAccount(ctx, tx, storage.Account{
		ID: systemAuditAccountID, Provider: systemAuditProvider, SubjectID: queueActorSystem,
		Login: systemAuditProvider, DisplayName: "Smyklot", UpdatedAt: request.ChangedAt,
	})
}

// Ordinary no-op saves roll back their read transaction. Imports still advance
// the comparison state, even when the settings already have the desired values.
func commitUnchangedConfigFileImport(
	ctx context.Context,
	tx *transaction,
	request storage.SaveInstallationSettingsRequest,
) error {
	if request.ConfigFileImport == nil {
		return nil
	}
	if err := writeConfigFileState(ctx, tx, request.ConfigFileImport.State); err != nil {
		return err
	}
	return tx.Commit()
}

func validateImportedSettingsWork(
	request storage.SaveInstallationSettingsRequest,
	work installationSettingsWork,
) error {
	if request.ConfigFileImport == nil {
		return nil
	}
	for _, item := range work.items {
		if item.After == nil {
			continue
		}
		if err := validateInstallationSettingsDocument(item.Kind, item.SyncKind, item.After.Document); err != nil {
			return err
		}
	}
	return nil
}

func validateSyncResourceRemovals(request storage.SaveInstallationSettingsRequest) error {
	if request.ConfigFileImport != nil {
		return nil
	}
	for _, item := range request.SyncConfigs {
		if item.Remove {
			return errors.New("removing sync resources requires a configuration file import")
		}
	}
	for _, item := range request.SyncOverrides {
		if item.Remove {
			return errors.New("removing sync resources requires a configuration file import")
		}
	}
	return nil
}
