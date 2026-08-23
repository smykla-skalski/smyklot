package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// MaxPathIndexInterval is as rarely as a repository's file list may be checked.
//
// The storage layer's, because the same bound is a CHECK constraint in both
// migration series and a value the browser is sent: one number, asserted
// against the SQL by `storage.TestPathIndexBound` and read off the wire by the
// panel rather than typed into it a fourth time.
const MaxPathIndexInterval = storage.MaxPathIndexInterval

type runtimeSettingsRequest struct {
	BotConfig                   requiredRuntimeValue[config.Config] `json:"bot_config"`
	LogLevel                    requiredRuntimeValue[string]        `json:"log_level"`
	PollIntervalSeconds         requiredRuntimeValue[int64]         `json:"reaction_poll_interval_seconds"`
	PendingCIQuietPeriodSeconds requiredRuntimeValue[int64]         `json:"merge_after_ci_quiet_period_seconds"`
	PathIndexIntervalSeconds    requiredRuntimeValue[int64]         `json:"path_index_interval_seconds"`
	SessionTTLSeconds           requiredRuntimeValue[int64]         `json:"session_ttl_seconds"`
	ExpectedRevision            requiredRuntimeValue[int64]         `json:"expected_revision"`
}

type requiredRuntimeValue[T any] struct {
	value   *T
	present bool
}

func (value *requiredRuntimeValue[T]) UnmarshalJSON(data []byte) error {
	value.present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.value = nil

		return nil
	}

	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.value = &decoded

	return nil
}

func (s *Server) getRootRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	settings, err := s.store.GetRuntimeSettings(r.Context())
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeSettingsDTO(
		settings, s.store.Status(r.Context()), s.cfg,
		s.runtimeValues(), s.startedAt, s.now().UTC(),
	))
}

func (s *Server) putRootRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	var input runtimeSettingsRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	change, proposed, err := s.runtimeSettingsChange(actor, input)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_runtime_settings", err.Error())
		return
	}
	effective, err := resolveRuntimeValues(s.cfg, proposed)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_runtime_settings", err.Error())
		return
	}
	change.EffectivePendingCIQuietPeriod = effective.PendingCIQuietPeriod
	change.EffectiveSessionTTL = effective.SessionTTL
	saved, err := s.store.SaveRuntimeSettings(r.Context(), change)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	updated := saved.Settings
	if saved.CheckpointID != nil {
		if err := s.applyRuntimeSettings(updated); err != nil {
			s.writeInternal(w, err)
			return
		}
		s.events.announce(panelEvent{Type: panelEventResync})
	}
	writeJSON(w, http.StatusOK, runtimeSettingsSaveDTO(
		saved, s.store.Status(r.Context()), s.cfg,
		s.runtimeValues(), s.startedAt, s.now().UTC(),
	))
}

func (s *Server) runtimeSettingsChange(
	actor storage.Account,
	input runtimeSettingsRequest,
) (storage.RuntimeSettingsChange, storage.RuntimeSettings, error) {
	if !completeRuntimeSettingsRequest(input) || input.ExpectedRevision.value == nil ||
		*input.ExpectedRevision.value < 0 {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{},
			fmt.Errorf("every runtime setting and expected revision is required")
	}
	botConfig, err := s.runtimeBotConfig(input.BotConfig.value)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	sessionTTL, err := runtimeDuration(
		input.SessionTTLSeconds.value,
		time.Minute,
		storage.MaxRuntimeSessionTTL,
		"session lifetime",
	)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	pollInterval, err := runtimeOptionalInterval(
		input.PollIntervalSeconds.value,
		storage.MinRuntimePollInterval,
		storage.MaxRuntimePollInterval,
		"reaction sweep interval",
	)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	pendingCIQuietPeriod, err := runtimeDuration(
		input.PendingCIQuietPeriodSeconds.value,
		pendingci.MinPassingQuiet,
		pendingci.MaxPassingQuiet,
		"merge-after-CI quiet period",
	)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	pathIndexInterval, err := runtimeDuration(
		input.PathIndexIntervalSeconds.value,
		0,
		MaxPathIndexInterval,
		"file list refresh interval",
	)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	if input.LogLevel.value != nil {
		if _, err := logging.ParseLevel(*input.LogLevel.value); err != nil {
			return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
		}
	}
	proposed := storage.RuntimeSettings{
		BotConfig: botConfig, LogLevel: input.LogLevel.value,
		PollInterval: pollInterval, PendingCIQuietPeriod: pendingCIQuietPeriod,
		SessionTTL: sessionTTL, PathIndexInterval: pathIndexInterval,
	}
	change := storage.RuntimeSettingsChange{
		BotConfig: botConfig, LogLevel: input.LogLevel.value,
		PollInterval: pollInterval, PendingCIQuietPeriod: pendingCIQuietPeriod,
		SessionTTL:        sessionTTL,
		PathIndexInterval: pathIndexInterval,
		ExpectedRevision:  *input.ExpectedRevision.value,
		ActorAccountID:    actor.ID, ChangedAt: s.now().UTC(),
	}

	return change, proposed, nil
}

func completeRuntimeSettingsRequest(input runtimeSettingsRequest) bool {
	return input.BotConfig.present && input.LogLevel.present && input.PollIntervalSeconds.present &&
		input.PendingCIQuietPeriodSeconds.present && input.PathIndexIntervalSeconds.present &&
		input.SessionTTLSeconds.present && input.ExpectedRevision.present
}

func runtimeOptionalInterval(
	seconds *int64,
	minimum, maximum time.Duration,
	label string,
) (*time.Duration, error) {
	if seconds == nil {
		return nil, nil
	}
	if *seconds == 0 {
		disabled := time.Duration(0)

		return &disabled, nil
	}

	return runtimeDuration(seconds, minimum, maximum, label)
}

func (s *Server) runtimeBotConfig(input *config.Config) (*config.Config, error) {
	if input == nil {
		return nil, nil
	}
	// Runner is deployment-owned rather than a panel setting. An omitted runner
	// is the canonical browser shape; an explicit value must still be valid.
	if input.Runner != "" {
		if _, err := config.ParseRunner(string(input.Runner)); err != nil {
			return nil, fmt.Errorf("invalid behavior defaults: %w", err)
		}
	}

	value := config.ApplyPatch(config.Default(), input.AsPatch())
	value.Runner = s.cfg.ProcessConfig.EffectiveRunner()

	return value, nil
}

func runtimeDuration(
	seconds *int64,
	minimum, maximum time.Duration,
	label string,
) (*time.Duration, error) {
	if seconds == nil {
		return nil, nil
	}
	if *seconds < 0 || *seconds > int64(maximum/time.Second) {
		return nil, fmt.Errorf("%s is outside the supported range", label)
	}
	value := time.Duration(*seconds) * time.Second
	if value != 0 && value < minimum {
		return nil, fmt.Errorf("%s must be at least %s", label, minimum)
	}
	if minimum > 0 && value == 0 {
		return nil, fmt.Errorf("%s must be at least %s", label, minimum)
	}

	return &value, nil
}
