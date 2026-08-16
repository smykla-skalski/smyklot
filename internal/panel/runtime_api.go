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

const (
	minRuntimePollInterval = time.Second
	maxRuntimePollInterval = 24 * time.Hour
	maxRuntimeSessionTTL   = 30 * 24 * time.Hour
)

type runtimeSettingsRequest struct {
	BotConfig           *config.Config `json:"bot_config"`
	LogLevel            *string        `json:"log_level"`
	PollIntervalSeconds *int64         `json:"reaction_poll_interval_seconds"`
	// This field needs presence as well as value: clients released before the
	// setting existed omit it, while an explicit null deliberately inherits.
	PendingCIQuietPeriodSeconds optionalRuntimeSeconds `json:"merge_after_ci_quiet_period_seconds"`
	SessionTTLSeconds           *int64                 `json:"session_ttl_seconds"`
	ExpectedRevision            *int64                 `json:"expected_revision"`
}

type optionalRuntimeSeconds struct {
	value   *int64
	present bool
}

func (value *optionalRuntimeSeconds) UnmarshalJSON(data []byte) error {
	value.present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.value = nil

		return nil
	}

	var seconds int64
	if err := json.Unmarshal(data, &seconds); err != nil {
		return err
	}
	value.value = &seconds

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
	var currentPendingCIQuietPeriod *time.Duration
	if !input.PendingCIQuietPeriodSeconds.present {
		current, err := s.store.GetRuntimeSettings(r.Context())
		if err != nil {
			s.writeStorageError(w, err)
			return
		}
		currentPendingCIQuietPeriod = current.PendingCIQuietPeriod
	}
	change, proposed, err := s.runtimeSettingsChange(
		actor,
		input,
		currentPendingCIQuietPeriod,
	)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_runtime_settings", err.Error())
		return
	}
	effective, err := resolveRuntimeValues(s.cfg, proposed)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_runtime_settings", err.Error())
		return
	}
	change.EffectivePollInterval = effective.PollInterval
	change.EffectivePendingCIQuietPeriod = effective.PendingCIQuietPeriod
	change.EffectiveSessionTTL = effective.SessionTTL
	updated, err := s.store.UpdateRuntimeSettings(r.Context(), change)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if err := s.applyRuntimeSettings(updated); err != nil {
		s.writeInternal(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventResync})
	writeJSON(w, http.StatusOK, runtimeSettingsDTO(
		updated, s.store.Status(r.Context()), s.cfg,
		s.runtimeValues(), s.startedAt, s.now().UTC(),
	))
}

func (s *Server) runtimeSettingsChange(
	actor storage.Account,
	input runtimeSettingsRequest,
	currentPendingCIQuietPeriod *time.Duration,
) (storage.RuntimeSettingsChange, storage.RuntimeSettings, error) {
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{},
			fmt.Errorf("expected revision is required")
	}
	botConfig, err := s.runtimeBotConfig(input.BotConfig)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	sessionTTL, err := runtimeDuration(
		input.SessionTTLSeconds, time.Minute, maxRuntimeSessionTTL, "session lifetime",
	)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	pollInterval, err := runtimeOptionalInterval(
		input.PollIntervalSeconds,
		minRuntimePollInterval,
		maxRuntimePollInterval,
		"reaction sweep interval",
	)
	if err != nil {
		return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
	}
	pendingCIQuietPeriod := currentPendingCIQuietPeriod
	if input.PendingCIQuietPeriodSeconds.present {
		pendingCIQuietPeriod, err = runtimeDuration(
			input.PendingCIQuietPeriodSeconds.value,
			pendingci.MinPassingQuiet,
			pendingci.MaxPassingQuiet,
			"merge-after-CI quiet period",
		)
		if err != nil {
			return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
		}
	}
	if input.LogLevel != nil {
		if _, err := logging.ParseLevel(*input.LogLevel); err != nil {
			return storage.RuntimeSettingsChange{}, storage.RuntimeSettings{}, err
		}
	}
	proposed := storage.RuntimeSettings{
		BotConfig: botConfig, LogLevel: input.LogLevel,
		PollInterval: pollInterval, PendingCIQuietPeriod: pendingCIQuietPeriod,
		SessionTTL: sessionTTL,
	}
	change := storage.RuntimeSettingsChange{
		BotConfig: botConfig, LogLevel: input.LogLevel,
		PollInterval: pollInterval, PendingCIQuietPeriod: pendingCIQuietPeriod,
		SessionTTL:       sessionTTL,
		ExpectedRevision: *input.ExpectedRevision,
		ActorAccountID:   actor.ID, ChangedAt: s.now().UTC(),
	}

	return change, proposed, nil
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
	// The runner is the process's, and is overwritten below whatever this
	// document says. A value that cannot mean anything is still refused rather
	// than silently replaced, so a typo is reported where it was made.
	if _, err := config.ParseRunner(string(input.Runner)); err != nil {
		return nil, fmt.Errorf("invalid behavior defaults: %w", err)
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
