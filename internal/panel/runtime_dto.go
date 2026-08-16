package panel

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type runtimeConfigValueResponse struct {
	Deployment config.Config  `json:"deployment"`
	Override   *config.Config `json:"override"`
	Effective  config.Config  `json:"effective"`
}

type runtimeDurationValueResponse struct {
	DeploymentSeconds int64  `json:"deployment_seconds"`
	OverrideSeconds   *int64 `json:"override_seconds"`
	EffectiveSeconds  int64  `json:"effective_seconds"`
}

type runtimeStringValueResponse struct {
	Deployment string  `json:"deployment"`
	Override   *string `json:"override"`
	Effective  string  `json:"effective"`
}

type runtimeServiceResponse struct {
	Version       string                 `json:"version"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	Storage       string                 `json:"storage"`
	Database      databaseStatusResponse `json:"database"`
	Listeners     struct {
		Public string `json:"public"`
		Admin  string `json:"admin"`
	} `json:"listeners"`
	PublicPaths struct {
		Panel   string `json:"panel"`
		Webhook string `json:"webhook"`
	} `json:"public_paths"`
	ProviderEndpoints struct {
		API       string `json:"api"`
		Authorize string `json:"authorize"`
		Token     string `json:"token"`
	} `json:"provider_endpoints"`
	Credentials struct {
		Webhook bool `json:"webhook"`
		App     bool `json:"app"`
		OAuth   bool `json:"oauth"`
	} `json:"credential_presence"`
}

type runtimeSettingsResponse struct {
	BehaviorDefaults     runtimeConfigValueResponse   `json:"behavior_defaults"`
	LogLevel             runtimeStringValueResponse   `json:"log_level"`
	PollInterval         runtimeDurationValueResponse `json:"reaction_poll_interval"`
	PendingCIQuietPeriod runtimeDurationValueResponse `json:"merge_after_ci_quiet_period"`
	SessionLifetime      runtimeDurationValueResponse `json:"session_lifetime"`
	Revision             int64                        `json:"revision"`
	UpdatedAt            *time.Time                   `json:"updated_at,omitempty"`
	UpdatedBy            *accountResponse             `json:"updated_by,omitempty"`
	Service              runtimeServiceResponse       `json:"service"`
}

func runtimeSettingsDTO(
	settings storage.RuntimeSettings,
	database storage.DatabaseStatus,
	cfg Config,
	effective RuntimeValues,
	startedAt, now time.Time,
) runtimeSettingsResponse {
	response := runtimeSettingsResponse{
		BehaviorDefaults: runtimeConfigValueResponse{
			Deployment: *cloneRuntimeConfig(cfg.ProcessConfig),
			Override:   cloneOptionalRuntimeConfig(settings.BotConfig),
			Effective:  *cloneRuntimeConfig(effective.BotConfig),
		},
		LogLevel: runtimeStringValueResponse{
			Deployment: runtimeLogLevelName(cfg.LogLevel),
			Override:   settings.LogLevel,
			Effective:  runtimeLogLevelName(effective.LogLevel),
		},
		PollInterval: runtimeDurationDTO(
			cfg.PollInterval, settings.PollInterval, effective.PollInterval,
		),
		PendingCIQuietPeriod: runtimeDurationDTO(
			cfg.PendingCIQuietPeriod,
			settings.PendingCIQuietPeriod,
			effective.PendingCIQuietPeriod,
		),
		SessionLifetime: runtimeDurationDTO(
			cfg.SessionTTL, settings.SessionTTL, effective.SessionTTL,
		),
		Revision:  settings.Revision,
		UpdatedAt: settings.UpdatedAt,
	}
	if settings.UpdatedBy != nil {
		updatedBy := accountDTO(*settings.UpdatedBy)
		response.UpdatedBy = &updatedBy
	}
	response.Service = runtimeServiceDTO(database, cfg, startedAt, now)

	return response
}

func runtimeDurationDTO(
	deployment time.Duration,
	override *time.Duration,
	effective time.Duration,
) runtimeDurationValueResponse {
	response := runtimeDurationValueResponse{
		DeploymentSeconds: int64(deployment / time.Second),
		EffectiveSeconds:  int64(effective / time.Second),
	}
	if override != nil {
		seconds := int64(*override / time.Second)
		response.OverrideSeconds = &seconds
	}

	return response
}

func runtimeServiceDTO(
	database storage.DatabaseStatus,
	cfg Config,
	startedAt, now time.Time,
) runtimeServiceResponse {
	databaseStatus := databaseStatusDTO(database)
	response := runtimeServiceResponse{
		Version:       cfg.Version,
		UptimeSeconds: max(int64(now.Sub(startedAt).Seconds()), 0),
		Storage:       databaseStatus.State,
		Database:      databaseStatus,
	}
	response.Listeners.Public = cfg.ListenAddress
	response.Listeners.Admin = cfg.AdminAddress
	response.PublicPaths.Panel = cfg.BasePath
	response.PublicPaths.Webhook = cfg.WebhookPath
	response.ProviderEndpoints.API = cfg.APIURL
	response.ProviderEndpoints.Authorize = cfg.AuthorizeURL
	response.ProviderEndpoints.Token = cfg.TokenURL
	response.Credentials.Webhook = cfg.WebhookCredentialPresent
	response.Credentials.App = cfg.AppCredentialPresent
	response.Credentials.OAuth = cfg.OAuthCredentialPresent

	return response
}

func cloneOptionalRuntimeConfig(value *config.Config) *config.Config {
	if value == nil {
		return nil
	}

	return cloneRuntimeConfig(value)
}
