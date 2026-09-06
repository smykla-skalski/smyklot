package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// DecodeExactJSON rejects field aliases that encoding/json would accept through
// case-insensitive struct matching. Reordering an object must never change which
// of "files" and "Files" takes effect. Arbitrary map keys remain case-sensitive.
// It also applies the duplicate-key, Unicode and size checks of DecodeJSONObject.
func DecodeExactJSON(document []byte, destination any) error {
	value := reflect.ValueOf(destination)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return errors.New("configuration JSON destination must be a non-nil pointer")
	}
	object, err := DecodeJSONObject(document)
	if err != nil {
		return err
	}
	if err := exactJSONFields(object, value.Type().Elem()); err != nil {
		return err
	}
	// Do not leave a partially decoded value behind when a later field fails.
	next := reflect.New(value.Type().Elem())
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(next.Interface()); err != nil {
		return err
	}
	value.Elem().Set(next.Elem())
	return nil
}

func exactJSONFields(value any, target reflect.Type) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	switch target.Kind() {
	case reflect.Struct:
		return exactJSONObjectFields(value, target)
	case reflect.Map:
		object, _ := value.(map[string]any)
		for _, child := range object {
			if err := exactJSONFields(child, target.Elem()); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		list, _ := value.([]any)
		for _, child := range list {
			if err := exactJSONFields(child, target.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func exactJSONStructFields(target reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for index := range target.NumField() {
		field := target.Field(index)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" || field.PkgPath != "" && !field.Anonymous {
			continue
		}
		underlying := field.Type
		for underlying.Kind() == reflect.Pointer {
			underlying = underlying.Elem()
		}
		if name == "" && field.Anonymous && underlying.Kind() == reflect.Struct {
			for key, child := range exactJSONStructFields(underlying) {
				fields[key] = child
			}
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func exactJSONObjectFields(value any, target reflect.Type) error {
	fields := exactJSONStructFields(target)
	object, _ := value.(map[string]any)
	for key, child := range object {
		field, found := fields[key]
		if !found {
			return fmt.Errorf("unknown configuration JSON field %.80q", key)
		}
		if err := exactJSONFields(child, field); err != nil {
			return fmt.Errorf("field %.80q: %w", key, err)
		}
	}
	return nil
}
