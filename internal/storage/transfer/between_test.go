package transfer_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
	"github.com/smykla-skalski/smyklot/internal/storage/transfer"
)

// TestBetweenCopiesByConnectionString covers the whole operation as an operator
// invokes it: two connection strings, and everything else decided underneath.
//
// Both sides are SQLite so this runs with no server, which matters because it
// is the path that resolves engines, migrates the destination and checks that
// what came back can be copied at all. A failure there would otherwise only
// show up on a machine with PostgreSQL running.
func TestBetweenCopiesByConnectionString(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	directory := t.TempDir()
	from := filepath.Join(directory, "from.db")
	to := filepath.Join(directory, "to.db")

	source, err := storagesqlite.Open(ctx, from)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := storagetest.Seed(ctx, source, seededAt); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	// Closed before the copy, because the copy opens it again. A SQLite file
	// held by two pools is exactly the situation the operator is told to avoid.
	if err := source.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	report, err := transfer.Between(ctx, from, to, transfer.Options{})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	requireSeededTablesCarriedRows(t, report)

	reopened, err := storagesqlite.Open(ctx, from)
	if err != nil {
		t.Fatalf("reopen source: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	copied, err := storagesqlite.Open(ctx, to)
	if err != nil {
		t.Fatalf("open destination: %v", err)
	}
	t.Cleanup(func() { _ = copied.Close() })

	requireSameState(t, ctx, reopened, copied)
}

// TestBetweenRejectsSameDatabase covers the mistake that would otherwise empty
// a database under --force and then copy it into itself.
func TestBetweenRejectsSameDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "same.db")
	_, err := transfer.Between(context.Background(), path, path, transfer.Options{})
	if err == nil {
		t.Fatal("copying a database into itself was allowed")
	}
}

// TestBetweenReportsAnUnusableConnection covers the operator typo. The message
// has to name which side was wrong, since both are connection strings and one
// of them may be a long DSN.
func TestBetweenReportsAnUnusableConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	good := filepath.Join(t.TempDir(), "good.db")

	_, err := transfer.Between(ctx, "mysql://nowhere/smyklot", good, transfer.Options{})
	if err == nil {
		t.Fatal("an unsupported source engine was accepted")
	}
	if !strings.Contains(err.Error(), "source") {
		t.Errorf("error does not say the source was at fault: %v", err)
	}

	_, err = transfer.Between(ctx, good, "mysql://nowhere/smyklot", transfer.Options{})
	if err == nil {
		t.Fatal("an unsupported destination engine was accepted")
	}
	if !strings.Contains(err.Error(), "destination") {
		t.Errorf("error does not say the destination was at fault: %v", err)
	}
	if errors.Is(err, transfer.ErrDestinationNotEmpty) {
		t.Errorf("an unopenable destination was reported as a populated one: %v", err)
	}
}
