package transfer_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	storagepostgres "github.com/smykla-skalski/smyklot/internal/storage/postgres"
	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
	"github.com/smykla-skalski/smyklot/internal/storage/transfer"
)

// dsnVariable names the server the copy runs against. There is no default, for
// the same reason the adapter's own suite has none: a test that invented a
// connection would either touch a database nobody meant it to, or pass by not
// running.
const dsnVariable = "SMYKLOT_TEST_POSTGRES_DSN"

// seededAt is the instant the fixture is written at. It is fixed rather than
// time.Now so that what the copy carries is the same on every run, and a
// timestamp that arrives wrong is a stable failure rather than a flake.
var seededAt = time.Date(2026, time.March, 14, 9, 30, 0, 0, time.UTC)

// TestCopyToPostgres proves an engine change keeps the data.
//
// It seeds SQLite through the port, copies to PostgreSQL, and then reads the
// result back through the port. Reading back is the part that matters: matching
// row counts only prove rows arrived, and the two engines disagree about how a
// timestamp, a boolean and a JSON document are spelled. A row that arrived with
// its config patch mangled or its expiry shifted counts the same and is still a
// broken migration.
func TestCopyToPostgres(t *testing.T) {
	t.Parallel()

	dsn := strings.TrimSpace(os.Getenv(dsnVariable))
	if dsn == "" {
		t.Skip(dsnVariable + " is not set, so there is no server to copy into")
	}

	ctx := context.Background()

	source, err := storagesqlite.Open(ctx, filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	if err := storagetest.Seed(ctx, source, seededAt); err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}

	destination := freshPostgres(t, ctx, dsn, "smyklot_copy")

	report, err := transfer.Copy(ctx, source, destination, transfer.Options{})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	requireSeededTablesCarriedRows(t, report)
	requireSameState(t, ctx, source, destination)
	requireWritesContinue(t, ctx, destination)
}

// requireWritesContinue writes to the copy, which is the only way to see
// whether the identity sequences moved.
//
// The copy writes each row's key directly, so PostgreSQL's own counter stays
// where the empty database left it. Without AfterCopy the next insert asks for
// key 1, finds the copied row already holding it, and the service fails on its
// first delivery - after a migration that reported success and read back
// perfectly. Every table below is keyed by a generated identity.
func requireWritesContinue(t *testing.T, ctx context.Context, store storage.Store) {
	t.Helper()

	after := seededAt.Add(12 * time.Minute)
	elevation := "seed-elevation"

	// audit_entries, app_audit_events and, because the write is elevated,
	// security_notifications.
	if _, err := store.UpdateTargetSettings(ctx, storage.TargetSettingsChange{
		TargetID:                 "github:installation:100",
		ActorAccountID:           "github:root",
		ElevationID:              &elevation,
		SessionTokenHash:         "seed-root-session",
		RepositoryDefaultEnabled: false,
		ExpectedRevision:         2,
		ChangedAt:                after,
	}); err != nil {
		t.Errorf("write after the copy: %v", err)
	}

	// access_audit_entries.
	role := storage.InstallationRoleViewer
	if _, err := store.SetTargetAccess(ctx, storage.TargetAccessChange{
		TargetID:         "github:installation:100",
		SubjectAccountID: "github:member",
		ActorAccountID:   "github:root",
		ElevationID:      &elevation,
		SessionTokenHash: "seed-root-session",
		Role:             &role,
		ExpectedRevision: 1,
		ChangedAt:        after,
	}); err != nil {
		t.Errorf("change access after the copy: %v", err)
	}

	// deliveries.
	if _, err := store.ClaimDelivery(ctx, storage.DeliveryClaim{
		ClaimKey:           "after:copy",
		DeliveryID:         "after-copy",
		TargetID:           "github:installation:100",
		RepositoryFullName: "smykla-skalski/smyklot",
		Event:              "issue_comment",
		ClaimedAt:          after,
	}); err != nil {
		t.Errorf("claim a delivery after the copy: %v", err)
	}
}

// TestCopyRefusesPopulatedDestination proves the guard that stops a copy from
// merging two histories. A second copy into the same database would duplicate
// what is already there, or fail halfway on a primary key it cannot see coming.
func TestCopyRefusesPopulatedDestination(t *testing.T) {
	t.Parallel()

	dsn := strings.TrimSpace(os.Getenv(dsnVariable))
	if dsn == "" {
		t.Skip(dsnVariable + " is not set, so there is no server to copy into")
	}

	ctx := context.Background()

	source, err := storagesqlite.Open(ctx, filepath.Join(t.TempDir(), "twice.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if err := storagetest.Seed(ctx, source, seededAt); err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}

	destination := freshPostgres(t, ctx, dsn, "smyklot_copy_twice")

	if _, err := transfer.Copy(ctx, source, destination, transfer.Options{}); err != nil {
		t.Fatalf("first copy: %v", err)
	}

	_, err = transfer.Copy(ctx, source, destination, transfer.Options{})
	if err == nil {
		t.Fatal("second copy into a populated database was allowed")
	}
	if !errors.Is(err, transfer.ErrDestinationNotEmpty) {
		t.Errorf("second copy failed for the wrong reason: %v", err)
	}

	// Force is the way through, and it has to leave the destination matching
	// the source rather than holding two of everything.
	forced, err := transfer.Copy(ctx, source, destination, transfer.Options{Force: true})
	if err != nil {
		t.Fatalf("forced copy: %v", err)
	}
	requireSeededTablesCarriedRows(t, forced)
	requireSameState(t, ctx, source, destination)
}

// requireSeededTablesCarriedRows fails when a table the fixture filled arrives
// empty, so a copy cannot pass by moving nothing.
func requireSeededTablesCarriedRows(t *testing.T, report transfer.Report) {
	t.Helper()

	for _, table := range storagetest.SeededTables() {
		if report.Rows[table] == 0 {
			t.Errorf("table %q was seeded but the copy carried no rows into it", table)
		}
	}
	if report.Total() == 0 {
		t.Fatal("the copy reported no rows at all")
	}
}

// requireSameState reads both databases through the port and compares what
// they answer. Every read goes through the same interface the application uses,
// so a difference here is a difference the application would see.
func requireSameState(t *testing.T, ctx context.Context, source, destination storage.Store) {
	t.Helper()

	checks := []struct {
		what string
		read func(storage.Store) (any, error)
	}{
		{"runtime settings", func(s storage.Store) (any, error) {
			return s.GetRuntimeSettings(ctx)
		}},
		{"root overview", func(s storage.Store) (any, error) {
			return s.GetRootOverview(ctx, "github:root", seededAt.Add(11*time.Minute))
		}},
		{"panel users", func(s storage.Store) (any, error) {
			return s.ListPanelUsers(ctx)
		}},
		{"root targets", func(s storage.Store) (any, error) {
			return s.ListRootTargets(ctx)
		}},
		{"repositories", func(s storage.Store) (any, error) {
			return s.ListRepositories(ctx, "github:installation:100")
		}},
		// What is known about each repository for each kind of sync, which is
		// the one table here holding a column that is empty in most rows: the
		// seed leaves one repository refused, and without reading the rows back
		// only the count of them was ever proven to survive the copy.
		{"sync repository state", func(s storage.Store) (any, error) {
			return s.ListSyncRepositoryState(ctx, "github:installation:100")
		}},
		{"session", func(s storage.Store) (any, error) {
			return s.GetSession(ctx, "seed-root-session", seededAt.Add(11*time.Minute))
		}},
		{"elevation", func(s storage.Store) (any, error) {
			return s.GetElevation(
				ctx, "seed-root-session", "github:installation:100", seededAt.Add(11*time.Minute),
			)
		}},
		{"preferences", func(s storage.Store) (any, error) {
			return s.GetPreferences(ctx, "github:root")
		}},
		{"invitation", func(s storage.Store) (any, error) {
			return s.GetInvitation(ctx, "seed-invitation", seededAt.Add(11*time.Minute))
		}},
		{"security notifications", func(s storage.Store) (any, error) {
			return s.ListSecurityNotifications(
				ctx, "github:1", storage.NotificationPageRequest{Limit: 50},
			)
		}},
		{"root audit", func(s storage.Store) (any, error) {
			return s.ListRootAudit(ctx, storage.RootAuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 50},
			})
		}},
		{"root failures", func(s storage.Store) (any, error) {
			return s.ListRootFailures(ctx, storage.FailurePageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 50},
			})
		}},
	}

	for _, check := range checks {
		want, wantErr := check.read(source)
		got, gotErr := check.read(destination)
		if fmt.Sprint(wantErr) != fmt.Sprint(gotErr) {
			t.Errorf("%s: sqlite returned %v, postgres returned %v", check.what, wantErr, gotErr)

			continue
		}
		// Compared as rendered values rather than with reflect.DeepEqual so a
		// failure names the field that differs. render follows pointers, which
		// is what makes the comparison mean anything: most of the interesting
		// fields on these models are optional, and %v would compare the
		// addresses two separate reads happened to allocate.
		if render(want) != render(got) {
			t.Errorf("%s differs after the copy:\n  sqlite:   %s\n  postgres: %s",
				check.what, render(want), render(got))
		}
	}
}

var timeType = reflect.TypeOf(time.Time{})

// render prints a value with every pointer followed and every timestamp in
// UTC, so two reads of the same state render identically whatever the engine
// underneath did with addresses and zones.
func render(value any) string {
	var out strings.Builder
	writeValue(&out, reflect.ValueOf(value))

	return out.String()
}

func writeValue(out *strings.Builder, value reflect.Value) {
	if !value.IsValid() {
		out.WriteString("<nil>")

		return
	}
	if value.Type() == timeType {
		stamp, ok := value.Interface().(time.Time)
		if ok {
			out.WriteString(stamp.UTC().Format(time.RFC3339Nano))

			return
		}
	}

	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			out.WriteString("<nil>")

			return
		}
		writeValue(out, value.Elem())
	case reflect.Struct:
		writeStruct(out, value)
	case reflect.Slice, reflect.Array:
		writeList(out, value)
	case reflect.Map:
		writeMap(out, value)
	default:
		fmt.Fprintf(out, "%v", value)
	}
}

func writeStruct(out *strings.Builder, value reflect.Value) {
	out.WriteByte('{')
	for index := range value.NumField() {
		if index > 0 {
			out.WriteByte(' ')
		}
		field := value.Type().Field(index)
		out.WriteString(field.Name)
		out.WriteByte(':')
		if !field.IsExported() {
			fmt.Fprintf(out, "%v", value.Field(index))

			continue
		}
		writeValue(out, value.Field(index))
	}
	out.WriteByte('}')
}

func writeList(out *strings.Builder, value reflect.Value) {
	if value.Kind() == reflect.Slice && value.IsNil() {
		out.WriteString("<nil>")

		return
	}
	// A byte slice is a JSON document everywhere these models use one, and
	// reads as one rather than as a list of numbers.
	if value.Type().Elem().Kind() == reflect.Uint8 {
		out.WriteString(string(value.Bytes()))

		return
	}
	out.WriteByte('[')
	for index := range value.Len() {
		if index > 0 {
			out.WriteByte(' ')
		}
		writeValue(out, value.Index(index))
	}
	out.WriteByte(']')
}

func writeMap(out *strings.Builder, value reflect.Value) {
	if value.IsNil() {
		out.WriteString("<nil>")

		return
	}
	// Sorted, because map iteration order is not, and an unordered render
	// would fail at random rather than when something is actually wrong.
	keys := make([]string, 0, value.Len())
	rendered := map[string]string{}
	iterator := value.MapRange()
	for iterator.Next() {
		var key strings.Builder
		writeValue(&key, iterator.Key())
		var entry strings.Builder
		writeValue(&entry, iterator.Value())
		keys = append(keys, key.String())
		rendered[key.String()] = entry.String()
	}
	sort.Strings(keys)

	out.WriteString("map[")
	for index, key := range keys {
		if index > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(key)
		out.WriteByte(':')
		out.WriteString(rendered[key])
	}
	out.WriteByte(']')
}

// freshPostgres migrates a store into a schema of its own, and drops it after.
func freshPostgres(
	t *testing.T,
	ctx context.Context,
	dsn, schema string,
) *storagepostgres.Store {
	t.Helper()

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	if _, err := admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		t.Fatalf("reset schema %s: %v", schema, err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.WithoutCancel(ctx), "DROP SCHEMA "+schema+" CASCADE")
	})

	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	store, err := storagepostgres.Open(ctx, dsn+separator+"search_path="+schema)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store
}
