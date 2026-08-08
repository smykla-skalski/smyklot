package sqlite

import (
	"net/url"
	"slices"
	"testing"
)

func TestDataSourceRequiresCrashDurableWALCommits(t *testing.T) {
	t.Parallel()

	dataSource, err := url.Parse(dataSourceName("/tmp/smyklot-panel-test.db"))
	if err != nil {
		t.Fatalf("parse SQLite data source: %v", err)
	}
	pragmas := dataSource.Query()["_pragma"]
	if !slices.Contains(pragmas, "journal_mode(WAL)") {
		t.Fatalf("SQLite pragmas do not enable WAL: %v", pragmas)
	}
	if !slices.Contains(pragmas, "synchronous(FULL)") {
		t.Fatalf("SQLite pragmas are not crash durable: %v", pragmas)
	}
}
