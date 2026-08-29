package panelcontract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panelcontract"
)

func TestGeneratedFrontendContractIsCurrent(t *testing.T) {
	t.Parallel()
	want, err := panelcontract.RenderTypeScript()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..")
	got, err := os.ReadFile(filepath.Join(root, panelcontract.FrontendFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("frontend render contract is stale; run `mise run generate`")
	}
}
