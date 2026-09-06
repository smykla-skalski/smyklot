package configsync

import "testing"

func present(document string) Snapshot {
	return Snapshot{Exists: true, Document: []byte(document)}
}

func TestReconcileBidirectionalChanges(t *testing.T) {
	cases := []struct {
		name                 string
		base, panel, file    string
		want                 string
		importPanel, publish bool
		advance              bool
		problem              Problem
	}{
		{"first missing file", "", `{"a":1}`, "", `{"a":1}`, false, true, false, ""},
		{"first empty file", "", `{"a":1}`, `{}`, `{"a":1}`, false, true, false, ""},
		{"first unrelated settings", "", `{"a":1}`, `{"b":2}`, `{"a":1,"b":2}`, true, true, false, ""},
		{"first differing value", "", `{"a":1}`, `{"a":2}`, "", false, false, false, ProblemConflictingEdits},
		{"already aligned", `{"a":1}`, `{"a":2}`, `{"a":2}`, `{"a":2}`, false, false, true, ""},
		{"panel edit", `{"a":1}`, `{"a":2}`, `{"a":1}`, `{"a":2}`, false, true, false, ""},
		{"file edit", `{"a":1}`, `{"a":1}`, `{"a":2}`, `{"a":2}`, true, false, false, ""},
		{"independent edits", `{"a":0,"b":0}`, `{"a":1,"b":0}`, `{"a":0,"b":2}`, `{"a":1,"b":2}`, true, true, false, ""},
		{"overlapping edits", `{"a":0}`, `{"a":1}`, `{"a":2}`, "", false, false, false, ProblemConflictingEdits},
		{"delete versus edit", `{"a":0}`, `{}`, `{"a":2}`, "", false, false, false, ProblemConflictingEdits},
		{"deleted file", `{"a":0}`, `{"a":0}`, "", "", false, false, false, ProblemFileRemoved},
		{"deleted empty file", `{}`, `{}`, "", "", false, false, false, ProblemFileRemoved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := ReconcileInput{Panel: []byte(tc.panel)}
			if tc.base != "" {
				input.Base = present(tc.base)
			}
			if tc.file != "" {
				input.File = present(tc.file)
			}
			decision, err := Reconcile(input)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Problem != tc.problem || decision.ImportPanel != tc.importPanel ||
				decision.PublishFile != tc.publish || decision.AdvanceBase != tc.advance {
				t.Fatalf("unexpected effects: %+v", decision)
			}
			if tc.problem != "" {
				if len(decision.Document) != 0 {
					t.Fatal("a blocked comparison must not supply publishable content")
				}
				return
			}
			equal, err := Equivalent(decision.Document, []byte(tc.want))
			if err != nil || !equal {
				t.Fatalf("document = %s, want %s (%v)", decision.Document, tc.want, err)
			}
		})
	}
}

func TestReconcileSurvivesImportBeforePRMerge(t *testing.T) {
	input := ReconcileInput{
		Base: present(`{"a":0,"b":0}`), Panel: []byte(`{"a":1,"b":0}`),
		File: present(`{"a":0,"b":2}`),
	}
	first, err := Reconcile(input)
	if err != nil {
		t.Fatal(err)
	}
	// A restart after importing the merged values, before the PR has merged,
	// must not report a false conflict or lose the still-pending panel edit.
	input.Panel = first.Document
	second, err := Reconcile(input)
	if err != nil || second.Problem != "" || second.ImportPanel || !second.PublishFile || second.AdvanceBase {
		t.Fatalf("retry before merge: %+v (%v)", second, err)
	}
	input.File = Snapshot{Exists: true, Document: second.Document}
	third, err := Reconcile(input)
	if err != nil || !third.AdvanceBase || third.ImportPanel || third.PublishFile {
		t.Fatalf("after merge: %+v (%v)", third, err)
	}
}

func TestResolutionIsBoundToComparedValues(t *testing.T) {
	input := ReconcileInput{
		Base: present(`{"a":0}`), Panel: []byte(`{"a":1}`), File: present(`{"a":2}`),
	}
	conflict, err := Reconcile(input)
	if err != nil || conflict.Problem != ProblemConflictingEdits {
		t.Fatalf("expected conflict: %+v (%v)", conflict, err)
	}
	input.Resolution = &Resolution{Comparison: conflict.Comparison, Document: []byte(`{"a":2}`)}
	resolved, err := Reconcile(input)
	if err != nil || resolved.Problem != "" || !resolved.ImportPanel || resolved.PublishFile {
		t.Fatalf("resolve to file: %+v (%v)", resolved, err)
	}
	for _, side := range []string{"panel", "file", "base", "presence"} {
		t.Run(side, func(t *testing.T) {
			changed := input
			switch side {
			case "panel":
				changed.Panel = []byte(`{"a":3}`)
			case "file":
				changed.File = present(`{"a":3}`)
			case "base":
				changed.Base = present(`{"a":3}`)
			case "presence":
				changed.File = Snapshot{}
			}
			decision, err := Reconcile(changed)
			if err != nil || decision.Problem != ProblemStaleResolution ||
				decision.ImportPanel || decision.PublishFile || decision.AdvanceBase {
				t.Fatalf("stale choice must have no effects: %+v (%v)", decision, err)
			}
		})
	}
}

func TestComparisonCanonicalizesWithoutTypeConfusion(t *testing.T) {
	key := func(document string) string {
		t.Helper()
		key, err := comparisonKey(ReconcileInput{Panel: []byte(document)})
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	if key(`{"a":1.00,"b":-0}`) != key(`{ "b":0, "a":1e0 }`) {
		t.Fatal("presentation alone changed the comparison")
	}
	for _, value := range []string{`"1"`, `["number","1e0"]`, `{"number":"1e0"}`} {
		if key(`{"a":1}`) == key(`{"a":`+value+`}`) {
			t.Fatalf("canonicalization conflated number and %s", value)
		}
	}
}
