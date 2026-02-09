// Package config provides environment variable loading for configuration structs.
//
// Environment variable names follow the pattern:
//
//	{Prefix}_{STAGE}_{FIELD}
//
// For named nested structs, the field name becomes a path segment:
//
//	{Prefix}_{STAGE}_{STRUCT}_{FIELD}
//
// Anonymous (embedded) struct fields are flattened and do not add a segment.
//
// Go field names are converted from CamelCase to UPPER_SNAKE_CASE:
//
//	BufferSize      → BUFFER_SIZE
//	ProcessTimeout  → PROCESS_TIMEOUT
//	RouterPool      → ROUTER_POOL
//
// Supported field types: string, bool, int*, uint*, float*, time.Duration.
// Fields with unsupported types (functions, interfaces, channels, pointers) are
// silently skipped.
//
// Example with pipe.Config and stage "transform":
//
//	GOPIPE_TRANSFORM_CONCURRENCY=5
//	GOPIPE_TRANSFORM_BUFFER_SIZE=10
//	GOPIPE_TRANSFORM_PROCESS_TIMEOUT=5s
//
// Example with message.EngineConfig and stage "engine":
//
//	GOPIPE_ENGINE_BUFFER_SIZE=100
//	GOPIPE_ENGINE_ROUTER_POOL_WORKERS=4
//	GOPIPE_ENGINE_PROCESS_TIMEOUT=30s
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var durationType = reflect.TypeOf(time.Duration(0))

// Loader reads environment variables into configuration structs.
type Loader struct {
	// Prefix for environment variable names.
	// Default: "GOPIPE".
	Prefix string

	// lookup overrides os.LookupEnv for testing.
	lookup func(string) (string, bool)
}

func (l Loader) prefix() string {
	if l.Prefix == "" {
		return "GOPIPE"
	}
	return l.Prefix
}

func (l Loader) lookupEnv(key string) (string, bool) {
	if l.lookup != nil {
		return l.lookup(key)
	}
	return os.LookupEnv(key)
}

// Load populates the struct pointed to by dst with values from environment
// variables. The stage parameter identifies the pipeline component and becomes
// the second segment of the variable name.
//
// Only fields with set environment variables are modified; all other fields
// retain their current values. This makes Load suitable for overlaying
// environment overrides on top of programmatic defaults.
func (l Loader) Load(stage string, dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: dst must be a pointer to a struct, got %T", dst)
	}
	prefix := l.prefix() + "_" + normalizeStage(stage)
	return l.loadStruct(prefix, v.Elem())
}

// Keys returns the environment variable names that [Loader.Load] would check
// for the given config struct. Useful for documentation and debugging.
// The dst parameter may be a struct value or a pointer to a struct.
func (l Loader) Keys(stage string, dst any) []string {
	v := reflect.ValueOf(dst)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	prefix := l.prefix() + "_" + normalizeStage(stage)
	return collectKeys(prefix, v.Type())
}

// Load populates dst using the default Loader with prefix "GOPIPE".
func Load(stage string, dst any) error {
	return Loader{}.Load(stage, dst)
}

// Keys returns env var names using the default Loader with prefix "GOPIPE".
func Keys(stage string, dst any) []string {
	return Loader{}.Keys(stage, dst)
}

func (l Loader) loadStruct(prefix string, v reflect.Value) error {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		fv := v.Field(i)

		// Unexported anonymous (embedded) struct fields are still recursed
		// into because their exported fields are promoted. All other
		// unexported fields are skipped.
		if !field.IsExported() {
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				if err := l.loadStruct(prefix, fv); err != nil {
					return err
				}
			}
			continue
		}

		// Build env var key: anonymous (embedded) structs are flattened,
		// named struct fields add their name as a path segment.
		var key string
		if field.Anonymous {
			key = prefix
		} else {
			key = prefix + "_" + toUpperSnake(field.Name)
		}

		// time.Duration is int64 underneath but should be parsed as "5s", "100ms".
		if field.Type == durationType {
			raw, ok := l.lookupEnv(key)
			if !ok {
				continue
			}
			d, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("config: %s: %w", key, err)
			}
			fv.SetInt(int64(d))
			continue
		}

		// Recurse into nested structs.
		if field.Type.Kind() == reflect.Struct {
			if err := l.loadStruct(key, fv); err != nil {
				return err
			}
			continue
		}

		// Skip unsupported types (func, interface, chan, ptr, etc.).
		if !isSupportedKind(field.Type.Kind()) {
			continue
		}

		raw, ok := l.lookupEnv(key)
		if !ok {
			continue
		}

		if err := setField(fv, raw, key); err != nil {
			return err
		}
	}
	return nil
}

func collectKeys(prefix string, t reflect.Type) []string {
	var keys []string
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				keys = append(keys, collectKeys(prefix, field.Type)...)
			}
			continue
		}

		var key string
		if field.Anonymous {
			key = prefix
		} else {
			key = prefix + "_" + toUpperSnake(field.Name)
		}

		if field.Type == durationType {
			keys = append(keys, key)
			continue
		}

		if field.Type.Kind() == reflect.Struct {
			keys = append(keys, collectKeys(key, field.Type)...)
			continue
		}

		if isSupportedKind(field.Type.Kind()) {
			keys = append(keys, key)
		}
	}
	return keys
}

func isSupportedKind(k reflect.Kind) bool {
	switch k {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func setField(v reflect.Value, raw, key string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("config: %s: %w", key, err)
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("config: %s: %w", key, err)
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("config: %s: %w", key, err)
		}
		v.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("config: %s: %w", key, err)
		}
		v.SetBool(b)
	}
	return nil
}

// normalizeStage converts a stage name to a valid env var segment.
// Lowercase letters are uppercased, hyphens/spaces/underscores become
// underscores, and other characters are dropped.
func normalizeStage(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(unicode.ToUpper(r))
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == ' ' || r == '_':
			b.WriteRune('_')
		}
	}
	return b.String()
}

// toUpperSnake converts a Go CamelCase field name to UPPER_SNAKE_CASE.
//
//	BufferSize     → BUFFER_SIZE
//	ProcessTimeout → PROCESS_TIMEOUT
//	URLPath        → URL_PATH
//	HTTPClient     → HTTP_CLIENT
func toUpperSnake(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4) // slightly over-allocate for underscores
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				b.WriteRune('_')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				b.WriteRune('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}
