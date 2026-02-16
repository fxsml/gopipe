// Package jsonschema provides a [Marshaler] that validates payloads
// against JSON Schema definitions.
//
// Schemas are the source of truth for data contracts. Go structs conform
// to schemas, not the other way around. This solves the Go zero-value
// dilemma: schema validation catches missing required fields before
// [encoding/json.Unmarshal] fills them with zero values.
//
// Register schemas per Go type at startup:
//
//	m := jsonschema.NewMarshaler()
//	m.MustRegister(CreateOrder{}, createOrderSchema)
//
// Raw schema JSON is available via [Marshaler.Schema] for HTTP serving:
//
//	w.Header().Set("Content-Type", "application/schema+json")
//	w.Write(m.Schema(CreateOrder{}))
package jsonschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	jschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Marshaler encodes and decodes JSON with per-type JSON Schema validation.
// It satisfies the message.Marshaler interface.
//
// Types without a registered schema pass through without validation.
type Marshaler struct {
	mu       sync.RWMutex
	compiler *jschema.Compiler
	schemas  map[reflect.Type]*entry
}

type entry struct {
	compiled *jschema.Schema
	raw      json.RawMessage
}

// NewMarshaler creates a JSON marshaler with schema validation.
func NewMarshaler() *Marshaler {
	return &Marshaler{
		compiler: jschema.NewCompiler(),
		schemas:  make(map[reflect.Type]*entry),
	}
}

// Register associates a JSON Schema with a Go type.
// The schema is compiled immediately. During marshal and unmarshal,
// values matching this type are validated against the schema.
func (m *Marshaler) Register(v any, schemaJSON string) error {
	t := elemType(v)
	uri := schemaURI(t)

	doc, err := jschema.UnmarshalJSON(strings.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("jsonschema: parsing schema for %s: %w", t, err)
	}
	if err := m.compiler.AddResource(uri, doc); err != nil {
		return fmt.Errorf("jsonschema: adding resource for %s: %w", t, err)
	}
	compiled, err := m.compiler.Compile(uri)
	if err != nil {
		return fmt.Errorf("jsonschema: compiling schema for %s: %w", t, err)
	}

	m.mu.Lock()
	m.schemas[t] = &entry{compiled: compiled, raw: json.RawMessage(schemaJSON)}
	m.mu.Unlock()

	return nil
}

// MustRegister is like Register but panics on error.
func (m *Marshaler) MustRegister(v any, schemaJSON string) {
	if err := m.Register(v, schemaJSON); err != nil {
		panic(err)
	}
}

// Schema returns the raw JSON Schema for a type, or nil if none is registered.
func (m *Marshaler) Schema(v any) json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e, ok := m.schemas[elemType(v)]; ok {
		return e.raw
	}
	return nil
}

// Marshal encodes v to JSON. If a schema is registered for the type,
// the output is validated before returning.
func (m *Marshaler) Marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if err := m.validate(v, data); err != nil {
		return nil, fmt.Errorf("marshal validation: %w", err)
	}
	return data, nil
}

// Unmarshal validates data against the schema for v's type, then decodes.
// Validation runs first: missing required fields are caught before Go
// fills them with zero values.
func (m *Marshaler) Unmarshal(data []byte, v any) error {
	if err := m.validate(v, data); err != nil {
		return fmt.Errorf("unmarshal validation: %w", err)
	}
	return json.Unmarshal(data, v)
}

// DataContentType returns "application/json".
func (m *Marshaler) DataContentType() string {
	return "application/json"
}

func (m *Marshaler) validate(v any, data []byte) error {
	t := elemType(v)

	m.mu.RLock()
	e, ok := m.schemas[t]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	inst, err := jschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return e.compiled.Validate(inst)
}

func elemType(v any) reflect.Type {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func schemaURI(t reflect.Type) string {
	return fmt.Sprintf("urn:gopipe:schema:%s/%s", t.PkgPath(), t.Name())
}
