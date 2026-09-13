package orgsync_test

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestRepositoryConfigurationDigestPreservesCacheIdentity(t *testing.T) {
	base := config.DefaultFormattingPolicy()
	changed := base
	changed.Common.IndentWidth++
	for _, kind := range orgsync.Kinds() {
		t.Run(string(kind), func(t *testing.T) {
			var inputs []orgsync.DigestInput
			if kind == orgsync.KindFiles {
				inputs = []orgsync.DigestInput{{Name: "formatting", Digest: orgsync.DigestFormattingPolicy(base)}}
			}
			want := orgsync.DigestRepositoryKindWithInputs("saved-config", nil, inputs)
			got := orgsync.DigestRepositoryConfiguration(kind, "saved-config", nil, base)
			if got != want {
				t.Fatal("changed the persisted cache identity")
			}
			reformatted := orgsync.DigestRepositoryConfiguration(kind, "saved-config", nil, changed)
			if (got != reformatted) != (kind == orgsync.KindFiles) {
				t.Fatal("formatting must affect files only")
			}
		})
	}
}
