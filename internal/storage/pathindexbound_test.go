package storage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// checkBound reads the upper end of a `BETWEEN 0 AND n` on a path-index column.
//
// Deliberately a regular expression over the SQL text rather than a query
// against a live database: the point is to catch a migration written with the
// wrong number, and an engine that has already accepted the file will answer
// happily about whatever bound it was given.
var checkBound = regexp.MustCompile(
	`path_index_interval_seconds[a-z_]* BETWEEN 0 AND (\d+)`,
)

// TestPathIndexBound asserts both migration series enforce the ceiling Go does.
//
// The bound lives in four places - this constant, a CHECK in each series, and
// the number the panel is sent - and the two SQL copies are the ones nothing
// else compares. A migration written with a different number is accepted by its
// engine and rejected by the service, or the reverse, and either way the two
// disagree about a value somebody typed into a box.
func TestPathIndexBound(t *testing.T) {
	t.Parallel()

	want := strconv.FormatInt(int64(storage.MaxPathIndexInterval/time.Second), 10)

	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			bounds := boundsIn(t, engine)

			// Three columns carry the setting: the runtime row, an installation
			// and a repository. A series that stopped constraining one of them
			// would otherwise pass here by having nothing to check.
			if len(bounds) != 3 {
				t.Errorf("found %d path-index CHECK bounds in %s, want 3", len(bounds), engine)
			}
			for path, bound := range bounds {
				if bound != want {
					t.Errorf(
						"%s bounds the file list refresh interval at %s seconds, "+
							"but storage.MaxPathIndexInterval is %s",
						path, bound, want,
					)
				}
			}
		})
	}
}

// boundsIn is every path-index CHECK bound in one engine's series, by the file
// it is written in - and by its position in that file, since 034 writes three.
func boundsIn(t *testing.T, engine string) map[string]string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(engine, "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("list %s migrations: %v", engine, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s migrations found - has the series moved?", engine)
	}

	bounds := map[string]string{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for at, match := range checkBound.FindAllStringSubmatch(string(body), -1) {
			bounds[fmt.Sprintf("%s (%d)", path, at+1)] = match[1]
		}
	}

	return bounds
}
