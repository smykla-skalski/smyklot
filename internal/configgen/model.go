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
	"slices"
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
	goInt    = "int"
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

	// ErrInvalidDefault reports a default or enum tag that does not fit the
	// field it sits on.
	ErrInvalidDefault = errors.New("invalid tag")

	// ErrDuplicateKey reports two fields addressed by the same key. Both would
	// render into one map entry, so one of them would silently never resolve.
	ErrDuplicateKey = errors.New("duplicate key")
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

	// KindInt is a bounded integer setting.
	KindInt Kind = "int"

	// KindObject is a nested sparse patch whose leaves are generated
	// individually while Config carries its complete policy counterpart.
	KindObject Kind = "object"
)

// Field is one setting, in every form the generated code and the schema need.
type Field struct {
	// GoName is the field's identifier, shared by Patch and Config.
	GoName string

	// ConstName is the Go suffix used for a leaf's generated key constant.
	ConstName string

	// Key is how the setting is addressed in a file, an environment variable
	// and the schema.
	Key string

	// GoType is the type Config carries: Patch's type with the pointer removed.
	GoType string

	// PatchType is the sparse struct type for a nested object.
	PatchType string

	// Kind decides the schema, the flag and the apply helper.
	Kind Kind

	// Named reports a setting whose Go type is one this package declares
	// rather than a builtin, such as Runner. Text read from an environment
	// variable or a flag has to go through that type's own parser, so the
	// generated code calls Parse<GoType> - which the compiler then insists
	// exists.
	Named bool

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

	// Presets are the non-default values a named preset assigns to this leaf.
	// Keeping these beside Default and Enum lets every consumer resolve the
	// same complete policy instead of reimplementing preset behaviour.
	Presets []PresetValue

	// Enum is the complete set of accepted values, for KindEnum.
	Enum []string

	// Min and Max bound an integer leaf. Empty means unbounded.
	Min string
	Max string

	// Children are the fields of a nested object. Leaves have none.
	Children []Field

	// GoPath names a leaf below its top-level Config/Patch field.
	GoPath []string

	// HasFlag reports whether the setting takes a command-line flag.
	HasFlag bool

	// PanelDeny reports a setting the panel must refuse to write.
	PanelDeny bool
}

// PresetValue is one named preset's value for a leaf.
type PresetValue struct {
	Name  string
	Value string
}

// Model is every setting, in the order Patch declares them. Declaration order
// is preserved rather than sorted, so the generated file reads like the source
// it came from and a reviewer can diff the two side by side.
type Model struct {
	Fields []Field
	Leaves []Field
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

	// Every file is read before anything is built, because a field's type may
	// be declared in a different one. Runner lives in types.go and Patch in
	// patch.go, and without the first the second cannot be told apart from a
	// field of some type the generator has no business rendering.
	var patch *ast.StructType

	stringTypes := make(map[string]struct{})
	structTypes := make(map[string]*ast.StructType)

	for _, name := range names {
		path := filepath.Join(dir, name)

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return Model{}, fmt.Errorf("parse %s: %w", path, err)
		}

		collectStringTypes(file, stringTypes)
		collectStructTypes(file, structTypes)

		if spec := findStruct(file, PatchTypeName); spec != nil && patch == nil {
			patch = spec
		}
	}

	if patch == nil {
		return Model{}, fmt.Errorf("%w in %s", ErrPatchNotFound, dir)
	}

	return build(patch, stringTypes, structTypes)
}

func collectStructTypes(file *ast.File, into map[string]*ast.StructType) {
	for _, decl := range file.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}

		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if value, ok := typeSpec.Type.(*ast.StructType); ok {
				into[typeSpec.Name.Name] = value
			}
		}
	}
}

// collectStringTypes records every `type X string` in the package.
//
// A named type is only renderable if it is a string underneath: the decoders
// read it from text, the schema publishes it as a string, and an enum tag
// closes the set. Without this the generator could not tell `*Runner` from
// `*int` - both are a bare identifier in the AST - and it took `*int` for a
// named string type, rendering a Config field that would not compile.
func collectStringTypes(file *ast.File, into map[string]struct{}) {
	for _, decl := range file.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}

		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Assign.IsValid() {
				continue
			}

			if ident, ok := typeSpec.Type.(*ast.Ident); ok && ident.Name == goString {
				into[typeSpec.Name.Name] = struct{}{}
			}
		}
	}
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

func build(
	spec *ast.StructType,
	stringTypes map[string]struct{},
	structTypes map[string]*ast.StructType,
) (Model, error) {
	var model Model

	seen := make(map[string]string)
	fields, err := buildFields(spec, nil, nil, stringTypes, structTypes, seen)
	if err != nil {
		return Model{}, err
	}

	model.Fields = fields
	model.Leaves = flattenLeaves(fields)

	return model, nil
}

func buildFields(
	spec *ast.StructType,
	keyPath, goPath []string,
	stringTypes map[string]struct{},
	structTypes map[string]*ast.StructType,
	seen map[string]string,
) ([]Field, error) {
	var fields []Field

	for _, astField := range spec.Fields.List {
		// An embedded field has no names. Patch has none, and one appearing
		// would be a setting with no key, so it is an error rather than a skip.
		if len(astField.Names) == 0 {
			return nil, fmt.Errorf("%w: embedded field", ErrUnsupportedType)
		}

		for _, name := range astField.Names {
			field, err := buildModelField(
				name.Name, astField, keyPath, goPath, stringTypes, structTypes, seen,
			)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name.Name, err)
			}
			fields = append(fields, field)
		}
	}

	return fields, nil
}

func buildModelField(
	name string,
	astField *ast.Field,
	keyPath, goPath []string,
	stringTypes map[string]struct{},
	structTypes map[string]*ast.StructType,
	seen map[string]string,
) (Field, error) {
	field, err := buildField(name, astField, stringTypes)
	if err != nil {
		return Field{}, err
	}

	field.GoPath = append(append([]string{}, goPath...), name)
	fullKeyPath := append(append([]string{}, keyPath...), field.Key)

	patchType, nested, isNested := nestedPatchType(astField.Type, structTypes)
	if isNested {
		if !strings.HasSuffix(patchType, "Patch") {
			return Field{}, fmt.Errorf("%w: nested type must end in Patch", ErrUnsupportedType)
		}
		field.Kind = KindObject
		field.PatchType = patchType
		field.GoType = strings.TrimSuffix(patchType, "Patch") + "Policy"
		field.Children, err = buildFields(
			nested, fullKeyPath, field.GoPath, stringTypes, structTypes, seen,
		)
		if err != nil {
			return Field{}, err
		}

		return field, nil
	}

	field.Key = strings.Join(fullKeyPath, ".")
	field.ConstName = strings.Join(field.GoPath, "")
	if owner, taken := seen[field.Key]; taken {
		return Field{}, fmt.Errorf("%w: %q also names %s", ErrDuplicateKey, field.Key, owner)
	}
	seen[field.Key] = name

	return field, nil
}

func nestedPatchType(
	expr ast.Expr,
	structTypes map[string]*ast.StructType,
) (string, *ast.StructType, bool) {
	pointer, ok := expr.(*ast.StarExpr)
	if !ok {
		return "", nil, false
	}
	ident, ok := pointer.X.(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	nested, ok := structTypes[ident.Name]
	if !ok {
		return "", nil, false
	}

	return ident.Name, nested, true
}

func flattenLeaves(fields []Field) []Field {
	var leaves []Field

	for _, field := range fields {
		if field.Kind == KindObject {
			leaves = append(leaves, flattenLeaves(field.Children)...)
			continue
		}

		leaves = append(leaves, field)
	}

	return leaves
}

func buildField(name string, astField *ast.Field, stringTypes map[string]struct{}) (Field, error) {
	pointer, ok := astField.Type.(*ast.StarExpr)
	if !ok {
		return Field{}, ErrFieldNotSparse
	}

	typed, err := classify(pointer.X, stringTypes)
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
		ConstName:   name,
		Key:         key,
		GoType:      typed.GoType,
		Kind:        typed.Kind,
		Named:       typed.Named,
		Doc:         doc,
		Description: describe(name, doc),
		Default:     tag.Get("default"),
		HasFlag:     tag.Get("flag") != "-",
		PanelDeny:   tag.Get("panel") == "deny",
		Min:         tag.Get("min"),
		Max:         tag.Get("max"),
	}

	if presets := tag.Get("presets"); presets != "" {
		field.Presets, err = parsePresets(presets)
		if err != nil {
			return Field{}, err
		}
	}

	if enum := tag.Get("enum"); enum != "" {
		field.Enum = strings.Split(enum, ",")
		field.Kind = KindEnum
	}

	if err := validate(field); err != nil {
		return Field{}, err
	}

	return field, nil
}

func parsePresets(raw string) ([]PresetValue, error) {
	parts := strings.Split(raw, ",")
	values := make([]PresetValue, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		name, value, found := strings.Cut(part, "=")
		if !found || name == "" || value == "" {
			return nil, fmt.Errorf("%w: malformed preset value %q", ErrInvalidDefault, part)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate preset %q", ErrInvalidDefault, name)
		}

		seen[name] = struct{}{}
		values = append(values, PresetValue{Name: name, Value: value})
	}

	return values, nil
}

// validate rejects a tag that would render into Go that does not compile, or
// worse, into Go that does.
//
// Without it a `default:"maybe"` on a boolean renders as the identifier `maybe`
// and the failure is a compile error in a generated file, which is a long way
// from the tag that caused it.
func validate(field Field) error {
	if err := validateDefault(field); err != nil {
		return err
	}
	for _, preset := range field.Presets {
		candidate := field
		candidate.Default = preset.Value
		if err := validateDefault(candidate); err != nil {
			return fmt.Errorf("preset %s: %w", preset.Name, err)
		}
	}

	// An enum is a closed set of strings. On a boolean it would silently
	// re-type the field and render a quoted default into a bool.
	if len(field.Enum) > 0 && field.GoType == goBool {
		return fmt.Errorf("%w: a boolean cannot carry an enum", ErrInvalidDefault)
	}

	return nil
}

func validateDefault(field Field) error {
	switch field.Kind {
	case KindBool:
		if field.Default != "" && field.Default != "true" && field.Default != "false" {
			return fmt.Errorf("%w: %q is not a boolean", ErrInvalidDefault, field.Default)
		}

	case KindEnum:
		if field.Default != "" && !slices.Contains(field.Enum, field.Default) {
			return fmt.Errorf("%w: %q is not one of %s",
				ErrInvalidDefault, field.Default, strings.Join(field.Enum, ", "))
		}

	case KindStringSlice, KindStringMap:
		if field.Default != "" {
			return fmt.Errorf("%w: a list or mapping defaults to empty", ErrInvalidDefault)
		}

	case KindInt:
		return validateIntegerDefault(field)
	}

	return nil
}

func validateIntegerDefault(field Field) error {
	value, err := strconv.Atoi(field.Default)
	if err != nil {
		return fmt.Errorf("%w: %q is not an integer", ErrInvalidDefault, field.Default)
	}
	if err := validateIntegerMinimum(value, field.Min); err != nil {
		return err
	}
	if field.Max == "" {
		return nil
	}
	maximum, err := strconv.Atoi(field.Max)
	if err != nil || value > maximum {
		return fmt.Errorf("%w: default %d is above max %q",
			ErrInvalidDefault, value, field.Max)
	}

	return nil
}

func validateIntegerMinimum(value int, raw string) error {
	if raw == "" {
		return nil
	}
	minimum, err := strconv.Atoi(raw)
	if err != nil || value < minimum {
		return fmt.Errorf("%w: default %d is below min %q", ErrInvalidDefault, value, raw)
	}

	return nil
}

// classified is what a Patch field's type tells the generator.
type classified struct {
	GoType string
	Kind   Kind
	Named  bool
}

// classify maps a Patch field's pointed-to type onto the Config type and the
// behaviour. An unrecognised type is refused rather than guessed at.
func classify(expr ast.Expr, stringTypes map[string]struct{}) (classified, error) {
	switch typed := expr.(type) {
	case *ast.Ident:
		switch typed.Name {
		case goBool:
			return classified{GoType: goBool, Kind: KindBool}, nil
		case goString:
			return classified{GoType: goString, Kind: KindString}, nil
		case goInt:
			return classified{GoType: goInt, Kind: KindInt}, nil
		default:
			// A named type such as Runner, accepted only when the package
			// declares it as a string. Anything else - int, a struct, a type
			// from another package - is refused, because the decoders read a
			// setting from text and the schema has to publish it as one.
			if _, ok := stringTypes[typed.Name]; !ok {
				if strings.HasSuffix(typed.Name, "Patch") {
					return classified{GoType: typed.Name, Kind: KindObject}, nil
				}
				return classified{}, fmt.Errorf(
					"%w: %s is not a string-based type declared in this package",
					ErrUnsupportedType, typed.Name)
			}

			return classified{GoType: typed.Name, Kind: KindString, Named: true}, nil
		}

	case *ast.ArrayType:
		if ident, ok := typed.Elt.(*ast.Ident); ok && ident.Name == goString && typed.Len == nil {
			return classified{GoType: "[]string", Kind: KindStringSlice}, nil
		}

	case *ast.MapType:
		key, keyOK := typed.Key.(*ast.Ident)
		value, valueOK := typed.Value.(*ast.Ident)

		if keyOK && valueOK && key.Name == goString && value.Name == goString {
			return classified{GoType: "map[string]string", Kind: KindStringMap}, nil
		}
	}

	return classified{}, ErrUnsupportedType
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
		return mapFirstRune(rest, unicode.ToUpper)
	}

	return doc
}

// mapFirstRune applies f to the first rune of text, leaving the rest alone.
//
// Decoding the rune rather than indexing a byte is what keeps a multi-byte
// first character intact: text[1:] would cut one apart.
func mapFirstRune(text string, f func(rune) rune) string {
	first, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return text
	}

	return string(f(first)) + text[size:]
}
