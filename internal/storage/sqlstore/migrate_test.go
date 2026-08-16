package sqlstore

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

// TestMigrationNamesRefuseTwoFilesAtOneVersion is the guard on a schema change
// that runs nowhere and says nothing.
//
// The runner records a version once and skips whatever repeats it, so of two
// files at one number the second is dropped in silence - and on a database that
// already ran one of them, the other is the one dropped instead. Two branches
// open at once is all it takes, and the schema the code expects is then not the
// schema the database has.
func TestMigrationNamesRefuseTwoFilesAtOneVersion(t *testing.T) {
	t.Parallel()

	_, err := migrationNames(fstest.MapFS{
		migrationDir + "/001_first.sql":           {Data: []byte("SELECT 1;")},
		migrationDir + "/002_one_branch.sql":      {Data: []byte("SELECT 1;")},
		migrationDir + "/002_the_other_one.sql":   {Data: []byte("SELECT 1;")},
		migrationDir + "/003_after_them_both.sql": {Data: []byte("SELECT 1;")},
	})

	if !errors.Is(err, errMigrationVersion) {
		t.Fatalf("err = %v, wanted a refusal naming the shared version", err)
	}

	// Both names, because whoever reads this has to know which two to look at.
	for _, name := range []string{"002_one_branch.sql", "002_the_other_one.sql"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal does not name %q: %v", name, err)
		}
	}
}

// TestMigrationNamesOrderThemByVersion keeps the ordinary case working: the
// files apply in the order their numbers say, not the order a directory listing
// happens to give.
func TestMigrationNamesOrderThemByVersion(t *testing.T) {
	t.Parallel()

	names, err := migrationNames(fstest.MapFS{
		migrationDir + "/002_second.sql": {Data: []byte("SELECT 1;")},
		migrationDir + "/001_first.sql":  {Data: []byte("SELECT 1;")},
		migrationDir + "/readme.md":      {Data: []byte("not a migration")},
	})
	if err != nil {
		t.Fatalf("a directory of ordinary migrations was refused: %v", err)
	}

	if len(names) != 2 || names[0] != "001_first.sql" || names[1] != "002_second.sql" {
		t.Errorf("names = %v, wanted the two .sql files in version order", names)
	}
}

// TestMigrationNamesRefuseAFileItCannotOrder covers the other way a directory
// stops describing a series: a file whose name carries no version at all.
func TestMigrationNamesRefuseAFileItCannotOrder(t *testing.T) {
	t.Parallel()

	_, err := migrationNames(fstest.MapFS{
		migrationDir + "/no-version-here.sql": {Data: []byte("SELECT 1;")},
	})

	if !errors.Is(err, errMigrationName) {
		t.Fatalf("err = %v, wanted a refusal naming the file", err)
	}
}
