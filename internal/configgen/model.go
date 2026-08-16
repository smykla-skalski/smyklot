// Package configgen reads config.Patch and writes everything derived from it.
//
// Patch is the one hand-written description of what Smyklot can be configured
// to do. Config, Default, the Key constants, applyPatch, the source map and the
// JSON Schema all say the same thing in different words, and before this package
// they said it five separate times - which is how command_aliases came to be
// spelled out in six places and Runner came to be missing from one of them.
//
// The generator is a pure function of the source: it parses the AST, builds a
// Model, and renders bytes. Nothing else runs, no formatter is shelled out to,
// and the output is deterministic, so `mise run generate` on a developer's
// machine produces the bytes CI checks for.
package configgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PatchTypeName is the type every generated artefact is derived from.
const PatchTypeName = "Patch"

// The Go type names classify recognises. They are spelled here rather than
// inline so the mapping reads as a table.
const (
	goBool   = "bool"
	goString = "string"
)

var (
	// ErrPatchNotFound reports a package with no Patch type in it.
	ErrPatchNotFound = errors.New("no " + PatchTypeName + " struct found")

	// ErrFieldNotSparse reports a Patch field that is not a pointer. Sparseness
	// is the whole point of the type: without it an omitted setting and an
	// explicit false are the same value.
	ErrFieldNotSparse = errors.New("field is not a pointer")

	// ErrUnsupportedType reports a Patch field the generator has no mapping for.
	// Failing here is deliberate: silently skipping it would produce a Config
	// that compiles and a schema that lies.
	ErrUnsupportedType = errors.New("unsupported field type")

	// ErrMissingKey reports a field with no key to address it by.
	ErrMissingKey = errors.New("field has no json tag")

	// ErrMissingDoc reports a field with no doc comment. The comment becomes the
	// schema description, so an undocumented field would publish an empty one.
	ErrMissingDoc = errors.New("field has no doc comment")
)

// Kind is how a setting behaves, which is what decides the Go type it takes,
// the JSON Schema it publishes, and the helper that applies it.
type Kind string

const (
	// KindBool is a flag that is either on or off.
	KindBool Kind = "bool"

	// KindString is free text.
	KindString Kind = "string"

	// KindEnum is text from a closed set.
	KindEnum Kind = "enum"

	// KindStringSlice is a list, replaced wholesale rather than merged.
	KindStringSlice Kind = "string_slice"

	// KindStringMap is a mapping, replaced wholesale rather than merged.
	KindStringMap Kind = "string_map"
)

// Field is one setting, in every form the generated code and the schema need.
type Field struct {
	// GoName is the field's identifier, shared by Patch and Config.
	GoName string

	// Key is how the setting is addressed in a file, an environment variable
	// and the schema.
	Key string

	// GoType is the type Config carries: Patch's type with the pointer removed.
	GoType string

	// Kind decides the schema, the flag and the apply helper.
	Kind Kind

	// Doc is the doc comment as written, which the generated Go field carries
	// so godoc reads the way the source did.
	Doc string

	// Description is the same comment with the leading identifier removed, so
	// an editor showing it against `quiet_success` reads a sentence about the
	// setting rather than about a Go field the reader cannot see.
	Description string

	// Default is the value Default() reports, in the canonical string form the
	// schema publishes. Empty means the type's zero value.
	Default string

	// Enum is the complete set of accepted values, for KindEnum.
	Enum []string

	// HasFlag reports whether the setting takes a command-line flag.
	HasFlag bool

	// PanelDeny reports a setting the panel must refuse to write.
	PanelDeny bool
}

// Model is every setting, in the order Patch declares them. Declaration order
// is preserved rather than sorted, so the generated file reads like the source
// it came from and a reviewer can diff the two side by side.
type Model struct {
	Fields []Field
}

// Keys returns every key, sorted, for the callers that need a stable set rather
// than declaration order.
func (m Model) Keys() []string {
	keys := make([]string, 0, len(m.Fields))
	for _, field := range m.Fields {
		keys = append(keys, field.Key)
	}
	sort.Strings(keys)

	return keys
}

// Parse reads the config package at dir and returns what Patch declares.
//
// The files are read in name order rather than through parser.ParseDir, which
// is deprecated and which ranges a map. Order only decides which duplicate
// declaration wins, and there is no duplicate, but a generator whose output
// depends on map iteration is one that eventually emits two different files
// from one input.
func Parse(dir string) (Model, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Model{}, fmt.Errorf("read %s: %w", dir, err)
	}

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	fset := token.NewFileSet()

	for _, name := range names {
		path := filepath.Join(dir, name)

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return Model{}, fmt.Errorf("parse %s: %w", path, err)
		}

		if spec := findStruct(file, PatchTypeName); spec != nil {
			return build(spec)
		}
	}

	return Model{}, fmt.Errorf("%w in %s", ErrPatchNotFound, dir)
}

func findStruct(file *ast.File, name string) *ast.StructType {
	var found *ast.StructType

	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != name {
			return true
		}
		if structType, ok := spec.Type.(*ast.StructType); ok {
			found = structType
		}

		return false
	})

	return found
}

func build(spec *ast.StructType) (Model, error) {
	var model Model

	for _, astField := range spec.Fields.List {
		// An embedded field has no names. Patch has none, and one appearing
		// would be a setting with no key, so it is an error rather than a skip.
		if len(astField.Names) == 0 {
			return Model{}, fmt.Errorf("%w: embedded field", ErrUnsupportedType)
		}

		for _, name := range astField.Names {
			field, err := buildField(name.Name, astField)
			if err != nil {
				return Model{}, fmt.Errorf("%s: %w", name.Name, err)
			}

			model.Fields = append(model.Fields, field)
		}
	}

	return model, nil
}

func buildField(name string, astField *ast.Field) (Field, error) {
	pointer, ok := astField.Type.(*ast.StarExpr)
	if !ok {
		return Field{}, ErrFieldNotSparse
	}

	goType, kind, err := classify(pointer.X)
	if err != nil {
		return Field{}, err
	}

	tag := parseTag(astField.Tag)

	key, _, _ := strings.Cut(tag.Get("json"), ",")
	if key == "" || key == "-" {
		return Field{}, ErrMissingKey
	}

	doc := strings.Join(strings.Fields(astField.Doc.Text()), " ")
	if doc == "" {
		return Field{}, ErrMissingDoc
	}

	field := Field{
		GoName:      name,
		Key:         key,
		GoType:      goType,
		Kind:        kind,
		Doc:         doc,
		Description: describe(name, doc),
		Default:     tag.Get("default"),
		HasFlag:     tag.Get("flag") != "-",
		PanelDeny:   tag.Get("panel") == "deny",
	}

	if enum := tag.Get("enum"); enum != "" {
		field.Enum = strings.Split(enum, ",")
		field.Kind = KindEnum
	}

	return field, nil
}

// classify maps a Patch field's pointed-to type onto the Config type and the
// behaviour. An unrecognised type is refused rather than guessed at.
func classify(expr ast.Expr) (string, Kind, error) {
	switch typed := expr.(type) {
	case *ast.Ident:
		switch typed.Name {
		case goBool:
			return goBool, KindBool, nil
		case goString:
			return goString, KindString, nil
		default:
			// A named type such as Runner. Its underlying type is not visible
			// from this file alone, so it is treated as a string and an enum
			// tag is what makes it a closed set.
			return typed.Name, KindString, nil
		}

	case *ast.ArrayType:
		if ident, ok := typed.Elt.(*ast.Ident); ok && ident.Name == goString && typed.Len == nil {
			return "[]string", KindStringSlice, nil
		}

	case *ast.MapType:
		key, keyOK := typed.Key.(*ast.Ident)
		value, valueOK := typed.Value.(*ast.Ident)

		if keyOK && valueOK && key.Name == goString && value.Name == goString {
			return "map[string]string", KindStringMap, nil
		}
	}

	return "", "", ErrUnsupportedType
}

func parseTag(literal *ast.BasicLit) reflect.StructTag {
	if literal == nil {
		return ""
	}

	unquoted, err := strconv.Unquote(literal.Value)
	if err != nil {
		return ""
	}

	return reflect.StructTag(unquoted)
}

// describe turns a Go doc comment into a description of the key.
//
// Go convention opens a comment with the identifier, which reads wrongly in an
// editor tooltip for `quiet_success`. Dropping the identifier and capitalising
// what follows leaves a sentence about the setting.
func describe(name, doc string) string {
	if rest, found := strings.CutPrefix(doc, name+" "); found {
		first, size := utf8.DecodeRuneInString(rest)
		if size > 0 {
			doc = string(unicode.ToUpper(first)) + rest[size:]
		}
	}

	return doc
}
