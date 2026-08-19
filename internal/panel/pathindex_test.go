package panel

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// The finder's answer is built twice - here, and by the dev mock, which is what
// a developer builds every panel change against. The two disagreed on exactly
// the fields nothing on screen states plainly: `repositories` is the
// denominator under "held by 4 of 6", and `observed_at` decides whether the
// notice above the finder says the list is stale. The mock counted every
// repository the installation has and stamped every answer with now(), so the
// notice could not appear in development at all.
//
// One table, two implementations, the way `filemerge` holds the composer.
// `internal/panel/frontend/tests/path-index.test.ts` runs the same file.
type pathIndexCase struct {
	Name string `json:"name"`
	Rows []struct {
		RepositoryID string    `json:"repository_id"`
		Paths        []string  `json:"paths"`
		ObservedAt   time.Time `json:"observed_at"`
		Partial      bool      `json:"partial"`
	} `json:"rows"`
	Expected json.RawMessage `json:"expected"`
}

func TestPathIndexAggregatesTheWayTheSharedTableSays(t *testing.T) {
	blob, err := os.ReadFile("testdata/path-index.json")
	if err != nil {
		t.Fatal(err)
	}

	var table struct {
		Cases []pathIndexCase `json:"cases"`
	}
	if err := json.Unmarshal(blob, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Cases) < 5 {
		t.Fatalf("the table has %d cases, which is too few to be the shared one", len(table.Cases))
	}

	for _, one := range table.Cases {
		t.Run(one.Name, func(t *testing.T) {
			rows := make([]orgsync.RepositoryPaths, 0, len(one.Rows))
			for _, row := range one.Rows {
				rows = append(rows, orgsync.RepositoryPaths{
					RepositoryID: row.RepositoryID,
					Paths:        row.Paths,
					ObservedAt:   row.ObservedAt,
					Partial:      row.Partial,
				})
			}

			// Compared as the JSON both sides send rather than as Go values:
			// what the browser reads is the wire, and a `time.Time` equal to
			// another `time.Time` says nothing about how each is written out.
			got, err := json.Marshal(syncPathIndex(rows))
			if err != nil {
				t.Fatal(err)
			}

			if same, err := sameJSON(got, one.Expected); err != nil {
				t.Fatal(err)
			} else if !same {
				t.Errorf("aggregated to\n  %s\nwanted\n  %s", got, one.Expected)
			}
		})
	}
}

// sameJSON compares two documents by what they mean rather than by their bytes,
// so key order and the table's own indentation are not part of the assertion.
func sameJSON(left, right []byte) (bool, error) {
	var one, two any
	if err := json.Unmarshal(left, &one); err != nil {
		return false, err
	}
	if err := json.Unmarshal(right, &two); err != nil {
		return false, err
	}

	first, err := json.Marshal(one)
	if err != nil {
		return false, err
	}
	second, err := json.Marshal(two)
	if err != nil {
		return false, err
	}

	return string(first) == string(second), nil
}
