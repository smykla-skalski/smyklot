package storage

import "testing"

func TestParseRepositoryID(t *testing.T) {
	t.Parallel()

	id, err := ParseRepositoryID(RepositoryID(31))
	if err != nil {
		t.Fatal(err)
	}
	if id != 31 {
		t.Fatalf("repository id = %d, want 31", id)
	}
}

func TestParseRepositoryIDRejectsInvalidStorageIdentities(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"31", "repository-31", "github:repository:", "github:repository:0"} {
		if _, err := ParseRepositoryID(id); err == nil {
			t.Errorf("ParseRepositoryID(%q) succeeded", id)
		}
	}
}
