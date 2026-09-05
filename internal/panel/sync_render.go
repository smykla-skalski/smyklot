package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filerender"
	"github.com/smykla-skalski/smyklot/internal/panelcontract"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type (
	syncFileRenderRequest           = panelcontract.SyncFileRenderInput
	syncFileRenderRepositoryRequest = panelcontract.SyncFileRenderRepositoryInput
	syncFileRenderDiagnostic        = panelcontract.SyncFileRenderDiagnostic
	syncFileFormattingLayerDTO      = panelcontract.SyncFileFormattingLayer
	syncFileFormattingResolutionDTO = panelcontract.SyncFileFormattingResolution
	syncFileRenderResponse          = panelcontract.SyncFileRenderResponse
)

type preparedSyncFileRender struct {
	Request   filerender.Request
	Inherited config.FormattingPolicy
	Current   config.Source
	Layers    []syncFileFormattingLayerDTO
}

func (s *Server) postSyncFileRender(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	var input syncFileRenderRequest
	if !decodeJSONWithin(w, r, &input, maxDocumentBody) {
		return
	}
	if input.Path == "" {
		writeJSON(w, http.StatusOK, invalidSyncFileRender(
			"request", "invalid_path", "a file path is required", nil,
		))

		return
	}
	if err := input.TemplateFormatting.Validate(); err != nil {
		writeJSON(w, http.StatusOK, invalidSyncFileRender(
			"policy", "invalid_policy", err.Error(), nil,
		))

		return
	}
	if input.Repository != nil {
		if input.Repository.ID == "" {
			writeJSON(w, http.StatusOK, invalidSyncFileRender(
				"request", "invalid_repository", "a repository id is required", nil,
			))

			return
		}
		if err := input.Repository.PathFormatting.Validate(); err != nil {
			writeJSON(w, http.StatusOK, invalidSyncFileRender(
				"policy", "invalid_policy", err.Error(), nil,
			))

			return
		}
	}

	prepared, diagnostic, err := s.prepareSyncFileRender(r, target, input)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	if diagnostic != nil {
		writeJSON(w, http.StatusOK, invalidSyncFileRender(
			diagnostic.Stage, diagnostic.Code, diagnostic.Message, nil,
		))

		return
	}

	rendered, err := filerender.Render(prepared.Request)
	resolution := &syncFileFormattingResolutionDTO{
		CurrentLayer: prepared.Current, InheritedPolicy: prepared.Inherited,
		EffectivePolicy: rendered.Resolved.Values.Formatting,
		Provenance:      rendered.Resolved.Formatting,
		Layers:          prepared.Layers,
	}
	if err != nil {
		stage := "format"
		var applyError *filemerge.ApplyError
		if errors.As(err, &applyError) {
			stage = string(applyError.Stage)
		}
		writeJSON(w, http.StatusOK, invalidSyncFileRender(
			stage, "invalid_document", err.Error(), resolution,
		))

		return
	}
	writeJSON(w, http.StatusOK, syncFileRenderResponse{
		Valid: true, FinalContent: string(rendered.Final),
		MatchesFormatting: rendered.MatchesFormatting,
		Diagnostics:       []syncFileRenderDiagnostic{}, Formatting: resolution,
	})
}

func (s *Server) prepareSyncFileRender(
	r *http.Request,
	target storage.Target,
	input syncFileRenderRequest,
) (preparedSyncFileRender, *syncFileRenderDiagnostic, error) {
	files, err := s.syncFileConfig(r, target.ID)
	if err != nil {
		return preparedSyncFileRender{}, nil, err
	}
	// A preview may be for a template staged in the same unsaved batch. Validate
	// its destination using the sync contract, but do not require persisted content.
	if err := (orgsync.FileConfig{Files: []orgsync.File{{Path: input.Path, Content: input.DraftContent}}}).Validate(); err != nil {
		return preparedSyncFileRender{}, &syncFileRenderDiagnostic{
			Stage: "request", Code: "invalid_document", Message: err.Error(),
		}, nil
	}
	managed, _ := managedSyncFile(files, input.Path)

	runtime := s.runtimeValues()
	configLayers := []config.Layer{{Source: config.SourceTarget, Patch: target.ConfigPatch}}
	dtoLayers := []syncFileFormattingLayerDTO{
		formattingLayerDTO(config.SourceProcess, "baseline",
			patchOrNil(runtime.BotConfig.Formatting.AsPatch())),
		formattingLayerDTO(config.SourceTarget, storedLayerState(target.ConfigPatch.Formatting),
			target.ConfigPatch.Formatting),
	}
	prepared := preparedSyncFileRender{
		Request: filerender.Request{
			Path: input.Path, Draft: []byte(input.DraftContent), Base: runtime.BotConfig,
		},
	}
	if input.Repository == nil {
		return prepareTemplateSyncFileRender(
			prepared, managed, input.TemplateFormatting, configLayers, dtoLayers,
		), nil, nil
	}

	return s.prepareRepositorySyncFileRender(
		r, target, prepared, managed, input.TemplateFormatting, *input.Repository,
		configLayers, dtoLayers,
	)
}

func prepareTemplateSyncFileRender(
	prepared preparedSyncFileRender,
	managed orgsync.File,
	formatting config.FormattingPatch,
	configLayers []config.Layer,
	dtoLayers []syncFileFormattingLayerDTO,
) preparedSyncFileRender {
	prepared.Inherited = config.Resolve(prepared.Request.Base, configLayers...).Values.Formatting
	configLayers = append(configLayers, formattingConfigLayer(config.SourceTemplate, formatting))
	dtoLayers = append(dtoLayers, formattingLayerDTO(
		config.SourceTemplate,
		draftLayerState(&formatting, managed.Formatting),
		patchOrNil(formatting),
	))
	prepared.Current = config.SourceTemplate
	prepared.Layers = dtoLayers
	prepared.Request.Layers = configLayers

	return prepared
}

func (s *Server) prepareRepositorySyncFileRender(
	r *http.Request,
	target storage.Target,
	prepared preparedSyncFileRender,
	managed orgsync.File,
	templateFormatting config.FormattingPatch,
	input syncFileRenderRepositoryRequest,
	configLayers []config.Layer,
	dtoLayers []syncFileFormattingLayerDTO,
) (preparedSyncFileRender, *syncFileRenderDiagnostic, error) {
	repository, err := s.store.GetRepository(r.Context(), target.ID, input.ID)
	if err != nil {
		return preparedSyncFileRender{}, nil, err
	}
	if !repository.Available {
		return preparedSyncFileRender{}, &syncFileRenderDiagnostic{
			Stage: "request", Code: "invalid_repository", Message: "repository not found",
		}, nil
	}
	if repository.IgnoreRepositoryFile {
		dtoLayers = append(dtoLayers, syncFileFormattingLayerDTO{
			Source: config.SourceRepositoryFile, State: panelcontract.LayerBypassed,
			ConfigPath: repository.ConfigFilePath,
		})
	} else {
		configLayers = append(configLayers, config.Layer{
			Source: config.SourceRepositoryFile, Patch: repository.ConfigFilePatch,
		})
		layer := formattingLayerDTO(
			config.SourceRepositoryFile,
			storedLayerState(repository.ConfigFilePatch.Formatting),
			repository.ConfigFilePatch.Formatting,
		)
		layer.ConfigPath = repository.ConfigFilePath
		dtoLayers = append(dtoLayers, layer)
	}
	configLayers = append(configLayers, config.Layer{
		Source: config.SourceRepositoryPanel, Patch: repository.ConfigPatch,
	})
	dtoLayers = append(dtoLayers, formattingLayerDTO(
		config.SourceRepositoryPanel,
		storedLayerState(repository.ConfigPatch.Formatting),
		repository.ConfigPatch.Formatting,
	))
	configLayers = append(configLayers, formattingConfigLayer(
		config.SourceTemplate, templateFormatting,
	))
	dtoLayers = append(dtoLayers, formattingLayerDTO(
		config.SourceTemplate,
		draftLayerState(&templateFormatting, managed.Formatting),
		patchOrNil(templateFormatting),
	))
	prepared.Inherited = config.Resolve(prepared.Request.Base, configLayers...).Values.Formatting

	storedPathFormatting, err := s.storedPathFormatting(
		r, target.ID, repository.ID, prepared.Request.Path,
	)
	if err != nil {
		return preparedSyncFileRender{}, nil, err
	}
	configLayers = append(configLayers, formattingConfigLayer(
		config.SourceRepositoryPath, input.PathFormatting,
	))
	dtoLayers = append(dtoLayers, formattingLayerDTO(
		config.SourceRepositoryPath,
		draftLayerState(&input.PathFormatting, storedPathFormatting),
		patchOrNil(input.PathFormatting),
	))

	prepared.Current = config.SourceRepositoryPath
	prepared.Layers = dtoLayers
	prepared.Request.Layers = configLayers
	prepared.Request.DefaultBranch = &repository.DefaultBranch
	if input.Merge != nil {
		prepared.Request.Merge = *input.Merge
	}

	return prepared, nil, nil
}

func managedSyncFile(files orgsync.FileConfig, wanted string) (orgsync.File, bool) {
	for _, file := range files.Files {
		if file.Path == wanted {
			return file, true
		}
	}

	return orgsync.File{}, false
}

func (s *Server) storedPathFormatting(
	r *http.Request,
	targetID, repositoryID, filePath string,
) (*config.FormattingPatch, error) {
	stored, err := s.store.GetSyncRepositoryOverride(
		r.Context(), targetID, repositoryID, orgsync.KindFiles,
	)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var override orgsync.FileOverride
	if err := json.Unmarshal(stored.Document, &override); err != nil {
		return nil, err
	}

	return override.FormattingFor(filePath), nil
}

func formattingConfigLayer(source config.Source, patch config.FormattingPatch) config.Layer {
	return config.Layer{Source: source, Patch: config.Patch{Formatting: &patch}}
}

func formattingLayerDTO(
	source config.Source,
	state panelcontract.LayerState,
	patch *config.FormattingPatch,
) syncFileFormattingLayerDTO {
	return syncFileFormattingLayerDTO{Source: source, State: state, Formatting: patch}
}

func patchOrNil(patch config.FormattingPatch) *config.FormattingPatch {
	if formattingPatchEmpty(&patch) {
		return nil
	}

	return &patch
}

func storedLayerState(patch *config.FormattingPatch) panelcontract.LayerState {
	if formattingPatchEmpty(patch) {
		return panelcontract.LayerAbsent
	}

	return panelcontract.LayerStored
}

func draftLayerState(draft, stored *config.FormattingPatch) panelcontract.LayerState {
	if reflect.DeepEqual(draft, stored) ||
		(formattingPatchEmpty(draft) && formattingPatchEmpty(stored)) {
		return storedLayerState(stored)
	}

	return panelcontract.LayerDraft
}

func formattingPatchEmpty(patch *config.FormattingPatch) bool {
	return patch == nil || reflect.DeepEqual(*patch, config.FormattingPatch{})
}

func invalidSyncFileRender(
	stage, code, message string,
	formatting *syncFileFormattingResolutionDTO,
) syncFileRenderResponse {
	return syncFileRenderResponse{
		Valid: false, Diagnostics: []syncFileRenderDiagnostic{{
			Stage: stage, Code: code, Message: message,
		}}, Formatting: formatting,
	}
}
