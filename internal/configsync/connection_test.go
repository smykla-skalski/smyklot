package configsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestConnectionCanReloadABaselineWhoseEnvelopeExceedsFileBudget(t *testing.T) {
	file := config.FileDocument{Panel: &config.PanelFileSection{
		Version: 1, Scope: config.PanelFileRepository,
		Settings: config.PanelFileSettings{ProtectedRefs: &config.PanelFileRefs{Include: []string{"refs/heads/" + strings.Repeat("x", 13<<20)}}},
	}}
	document, err := config.EncodeJSONDocument(file)
	if err != nil {
		t.Fatal(err)
	}
	connection := Connection{Version: 1, Status: StatusReady, Base: Snapshot{Exists: true, Document: document}}
	snapshot := completeSnapshot(config.PanelFileRepository)
	change, err := connection.change(snapshot, storage.ConfigFileState{}, time.Now().UTC())
	if err != nil || len(change.Document) <= config.MaxFileDocumentBytes {
		t.Fatalf("test did not exercise a valid large state wrapper: %d (%v)", len(change.Document), err)
	}
	restored, err := DecodeConnection(storage.ConfigFileState{Document: change.Document}, config.PanelFileRepository)
	if err != nil || restored.Status != StatusReady || !bytes.Equal(restored.Base.Document, document) {
		t.Fatalf("valid large baseline cannot be read back: %v", err)
	}
	var ordinary map[string]any
	if err := config.DecodeExactJSON(change.Document, &ordinary); err == nil {
		t.Fatal("the larger envelope budget weakened the normal file limit")
	}
}

func TestConnectionRejectsOversizedOrAmbiguousEnvelopes(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"Version":2}`, `{"version":1,"version":2}`, `{"version":1,"unknown":true}`,
		`{"version":1,"message":"\ud800"}`, `{"version":2}`, `{"version":1,"resolution":{"side":"anything"}}`,
	} {
		if _, err := DecodeConnection(storage.ConfigFileState{Document: []byte(raw)}, config.PanelFileRepository); err == nil {
			t.Errorf("accepted invalid connection: %s", raw)
		}
	}
	raw, _ := json.Marshal(Connection{Version: 1, Message: strings.Repeat("x", storage.MaxConfigFileStateBytes)})
	if _, err := DecodeConnection(storage.ConfigFileState{Document: raw}, config.PanelFileRepository); err == nil {
		t.Fatal("accepted connection beyond its storage bound")
	}
}

func TestConnectionChoicesPreserveIndependentChanges(t *testing.T) {
	input := ReconcileInput{
		Base:  present(`{"shared":0,"removed":true}`),
		Panel: []byte(`{"shared":1,"panel_only":9007199254740993}`),
		File:  present(`{"shared":2,"removed":true,"file_only":null}`),
	}
	comparison, err := comparisonKey(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"panel", "file"} {
		connection := Connection{Base: input.Base, Resolution: &ResolutionChoice{Side: side, Comparison: comparison}}
		bound, err := connection.input(input.Panel, FileSource{Snapshot: input.File})
		if err != nil {
			t.Fatal(err)
		}
		decision, err := Reconcile(bound)
		if err != nil || decision.Problem != "" {
			t.Fatalf("choice did not resolve: %+v (%v)", decision, err)
		}
		want := `{"shared":1,"panel_only":9007199254740993,"file_only":null}`
		if side == "file" {
			want = `{"shared":2,"panel_only":9007199254740993,"file_only":null}`
		}
		if same, err := Equivalent(decision.Document, []byte(want)); err != nil || !same {
			t.Fatalf("%s choice discarded an independent edit: %s (%v)", side, decision.Document, err)
		}
	}
}

func TestConnectionBlocksAResolvedDocumentBeyondTheFileLimit(t *testing.T) {
	value := strings.Repeat("x", config.MaxFileDocumentBytes/2)
	panel, _ := json.Marshal(map[string]string{"panel": value})
	file, _ := json.Marshal(map[string]string{"file": value})
	_, err := resolvedContent(ReconcileInput{Panel: panel, File: Snapshot{Exists: true, Document: file}}, "panel")
	var blocked *BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "invalid_settings" {
		t.Fatalf("oversized combined settings must stop retries and request correction: %v", err)
	}
}
