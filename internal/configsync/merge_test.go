package configsync

import (
	"reflect"
	"testing"
)

type mergeTestCase struct {
	name, base, panel, file, want string
	paths                         [][]string
}

func TestMergeIndependentSettings(t *testing.T) {
	cases := []mergeTestCase{
		{"unchanged", `{"a":1}`, `{"a":1.0}`, `{"a":1e0}`, `{"a":1}`, nil},
		{"unbounded exponent", `{"a":1e999999999999}`, `{"a":10e999999999998}`, `{"a":1.0e999999999999}`, `{"a":1e999999999999}`, nil},
		{"independent fields", `{"a":1,"b":1}`, `{"a":2,"b":1}`, `{"a":1,"b":2}`, `{"a":2,"b":2}`, nil},
		{"same edit", `{"a":1}`, `{"a":2}`, `{"a":2}`, `{"a":2}`, nil},
		{"delete and add", `{"a":false}`, `{}`, `{"a":false,"b":0}`, `{"b":0}`, nil},
		{"independent new object", `{}`, `{"settings":{"a":1}}`, `{"settings":{"b":2}}`, `{"settings":{"a":1,"b":2}}`, nil},
		{"null is explicit", `{"a":null}`, `{}`, `{"a":1}`, `{}`, [][]string{{"a"}}},
		{"delete versus edit", `{"a":{"b":1}}`, `{}`, `{"a":{"b":2}}`, `{}`, [][]string{{"a"}}},
		{"lists conflict", `{"a":[1]}`, `{"a":[1,2]}`, `{"a":[0,1]}`, `{"a":[1,2]}`, [][]string{{"a"}}},
		{"nested and dotted keys", `{"a.b":{"c":1},"z":0}`, `{"a.b":{"c":2},"z":1}`, `{"a.b":{"c":3},"z":2}`, `{"a.b":{"c":2},"z":1}`, [][]string{{"a.b", "c"}, {"z"}}},
		{"large integers stay distinct", `{"a":9007199254740992}`, `{"a":9007199254740993}`, `{"a":9007199254740994}`, `{"a":9007199254740993}`, [][]string{{"a"}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			checkMergeCase(t, test)
		})
	}
}

func checkMergeCase(t *testing.T, test mergeTestCase) {
	t.Helper()
	result, err := Merge([]byte(test.base), []byte(test.panel), []byte(test.file))
	if err != nil {
		t.Fatal(err)
	}
	same, err := Equivalent(result.Document, []byte(test.want))
	if err != nil || !same {
		t.Fatalf("got %s, want %s: %v", result.Document, test.want, err)
	}
	var paths [][]string
	for _, conflict := range result.Conflicts {
		paths = append(paths, conflict.Path)
	}
	if !reflect.DeepEqual(paths, test.paths) {
		t.Fatalf("conflicts %v, want %v", paths, test.paths)
	}
	// The set of conflicts and conflict-free answer are direction-independent.
	reverse, err := Merge([]byte(test.base), []byte(test.file), []byte(test.panel))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reverse.Conflicts, result.Conflicts) {
		t.Fatal("direction changed conflicts")
	}
	if len(paths) == 0 {
		same, _ := Equivalent(reverse.Document, result.Document)
		if !same {
			t.Fatal("direction changed merged settings")
		}
	}
}

func TestMergeRejectsInvalidDocuments(t *testing.T) {
	for _, invalid := range []string{"", "null", "[]", "{", "{} {}"} {
		for index := range 3 {
			inputs := [][]byte{[]byte("{}"), []byte("{}"), []byte("{}")}
			inputs[index] = []byte(invalid)
			if _, err := Merge(inputs[0], inputs[1], inputs[2]); err == nil {
				t.Fatalf("accepted invalid input %q at %d", invalid, index)
			}
		}
	}
}
