package panel

import (
	"log/slog"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestDatabaseStatusDTOCarriesWhatTheEngineReported(t *testing.T) {
	response := databaseStatusDTO(healthyDatabaseStatus())

	if response.State != rootServiceHealthy {
		t.Fatalf("state = %q, want %q", response.State, rootServiceHealthy)
	}
	if response.Engine != "PostgreSQL" || response.Version != "18.6" {
		t.Fatalf("engine = %q, version = %q", response.Engine, response.Version)
	}
	if response.SchemaVersion != 1 || response.SizeBytes != 84_711_103 {
		t.Fatalf("schema = %d, size = %d", response.SchemaVersion, response.SizeBytes)
	}
	if response.Detail != "" {
		t.Fatalf("detail = %q on a database that answered everything", response.Detail)
	}

	// 1,234µs, not 1ms and not 1.234ms: a round trip is worth reporting to
	// hundredths of a millisecond, and no finer, or the number changes on
	// every read of a page that has not changed.
	if response.LatencyMillis != 1.23 {
		t.Fatalf("latency = %v ms, want 1.23", response.LatencyMillis)
	}

	connections := response.Connections
	if connections.Open != 3 || connections.InUse != 1 ||
		connections.Idle != 2 || connections.Max != 16 {
		t.Fatalf("connections = %#v", connections)
	}
	if connections.WaitCount != 7 || connections.WaitMillis != 250 {
		t.Fatalf("waits = %d over %v ms", connections.WaitCount, connections.WaitMillis)
	}
}

func TestDatabaseStatusDTODerivesTheStateFromReachability(t *testing.T) {
	degraded := healthyDatabaseStatus()
	degraded.Error = "relation \"schema_migrations\" does not exist"

	response := databaseStatusDTO(degraded)
	if response.State != rootServiceDegraded {
		t.Fatalf("a reachable database that would not describe itself = %q", response.State)
	}
	if response.Detail != degraded.Error {
		t.Fatalf("detail = %q, want the reason it is degraded", response.Detail)
	}

	unreachable := storage.DatabaseStatus{Engine: "PostgreSQL", Error: "connection refused"}
	if response := databaseStatusDTO(unreachable); response.State != rootServiceUnavailable {
		t.Fatalf("an unreachable database = %q", response.State)
	}

	// A full pool is a busy instant, not a fault. Colouring it would put the
	// page in a state an operator cannot act on and cannot clear.
	saturated := healthyDatabaseStatus()
	saturated.Connections.InUse = saturated.Connections.Max
	saturated.Connections.Idle = 0
	saturated.Connections.Open = saturated.Connections.Max
	if response := databaseStatusDTO(saturated); response.State != rootServiceHealthy {
		t.Fatalf("a fully used pool = %q, want it left alone", response.State)
	}
}

func TestServiceResponsesSummarizeTheSameDatabaseTheyDescribe(t *testing.T) {
	status := healthyDatabaseStatus()
	status.Error = "could not read database size"
	cfg := Config{Version: "1.0.0", LogLevel: slog.LevelInfo, ProcessConfig: config.Default()}
	startedAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	now := startedAt.Add(time.Minute)

	// Two responses carry the state twice, once as a word beside the service
	// and once inside the database block. Reading it from the same DTO is what
	// stops a card saying healthy over a panel that says otherwise.
	overview := rootOverviewDTO(storage.RootOverview{}, status, nil, nil, nil, cfg, startedAt, now)
	if overview.Service.Storage != rootServiceDegraded {
		t.Fatalf("overview storage = %q", overview.Service.Storage)
	}
	if overview.Service.Storage != overview.Service.Database.State {
		t.Fatalf(
			"overview says %q beside %q",
			overview.Service.Storage, overview.Service.Database.State,
		)
	}

	settings := runtimeSettingsDTO(
		storage.RuntimeSettings{}, status, cfg,
		RuntimeValues{BotConfig: config.Default(), PollInterval: time.Minute, SessionTTL: time.Hour},
		startedAt, now,
	)
	if settings.Service.Storage != rootServiceDegraded {
		t.Fatalf("settings storage = %q", settings.Service.Storage)
	}
	if settings.Service.Storage != settings.Service.Database.State {
		t.Fatalf(
			"settings say %q beside %q",
			settings.Service.Storage, settings.Service.Database.State,
		)
	}
	if settings.Service.Database.Engine != status.Engine {
		t.Fatalf("settings engine = %q", settings.Service.Database.Engine)
	}
}

func healthyDatabaseStatus() storage.DatabaseStatus {
	return storage.DatabaseStatus{
		Engine:        "PostgreSQL",
		Version:       "18.6",
		SchemaVersion: 1,
		SizeBytes:     84_711_103,
		Reachable:     true,
		Latency:       1234 * time.Microsecond,
		Connections: storage.ConnectionStats{
			Open: 3, InUse: 1, Idle: 2, Max: 16,
			WaitCount: 7, WaitDuration: 250 * time.Millisecond,
		},
	}
}
