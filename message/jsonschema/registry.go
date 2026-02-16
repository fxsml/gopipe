// Package jsonschema provides schema validation for CloudEvents messages.
//
// Registry validates message payloads against JSON Schema definitions.
// Schemas are the source of truth for data contracts. This solves the Go
// zero-value dilemma: validation catches missing required fields before
// encoding/json.Unmarshal fills them with zero values.
//
// Register schemas by CloudEvents type:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{
//	    Naming: message.KebabNaming,
//	})
//	registry.MustRegisterType(CreateOrderCommand{}, commandSchema)
//	// → registers as "create.order.command" with KebabNaming
//
// Validation middleware:
//
//	validator := jsonschema.NewValidationMiddleware(registry)
//	pipe.Use(validator)
//
// Schema serving:
//
//	w.Header().Set("Content-Type", "application/schema+json")
//	w.Write(registry.Schemas())  // All schemas as catalog
//	w.Write(registry.Schema("order.created"))  // Individual schema
package jsonschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	jschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/fxsml/gopipe/message"
)

// Config configures the schema registry.
type Config struct {
	// Naming strategy for deriving CloudEvents types from Go types.
	// Used by RegisterType to automatically convert Go type names to event types.
	// Default: message.KebabNaming (CreateOrderCommand → "create.order.command")
	Naming message.EventTypeNaming

	// SchemaURI generates a unique URI for schema compilation.
	// Optional. Defaults to urn:gopipe:schema:cloudevents:{eventType}
	//
	// The URI is used internally by the JSON Schema compiler and doesn't
	// need to be resolvable. However, using HTTP URLs allows schemas to
	// be published at those locations for external tooling.
	//
	// Example custom URIs:
	//   - https://api.example.com/schemas/{eventType}
	//   - urn:mycompany:events:{eventType}:schema
	SchemaURI func(eventType string) string
}

// Registry validates message payloads against JSON Schema definitions.
// It stores schemas by CloudEvents type and optionally tracks Go types
// for convenience registration and InputRegistry implementation.
//
// Registry is thread-safe and can be shared across middleware and pipes.
type Registry struct {
	mu        sync.RWMutex
	compiler  *jschema.Compiler
	schemas   map[string]*entry          // eventType → schema
	types     map[string]reflect.Type    // eventType → Go type (for InputRegistry)
	reverse   map[reflect.Type]string    // Go type → eventType (for RegisterType)
	naming    message.EventTypeNaming
	schemaURI func(string) string        // eventType → URI for compiler
}

type entry struct {
	compiled *jschema.Schema
	raw      json.RawMessage
}

// NewRegistry creates a schema validation registry.
// If cfg.Naming is nil, uses KebabNaming by default.
// If cfg.SchemaURI is nil, uses defaultSchemaURI.
func NewRegistry(cfg Config) *Registry {
	if cfg.Naming == nil {
		cfg.Naming = message.KebabNaming
	}
	if cfg.SchemaURI == nil {
		cfg.SchemaURI = defaultSchemaURI
	}
	return &Registry{
		compiler:  jschema.NewCompiler(),
		schemas:   make(map[string]*entry),
		types:     make(map[string]reflect.Type),
		reverse:   make(map[reflect.Type]string),
		naming:    cfg.Naming,
		schemaURI: cfg.SchemaURI,
	}
}

// RegisterType associates a JSON Schema with a Go type.
// The CloudEvents type is derived using the naming strategy.
//
// Example:
//
//	registry.RegisterType(CreateOrderCommand{}, commandSchema)
//	// With KebabNaming: CreateOrderCommand → "create.order.command"
//
// The Go type is tracked for InputRegistry.NewInput() support.
func (r *Registry) RegisterType(v any, schemaJSON string) error {
	t := elemType(v)
	eventType := r.naming.EventType(t)
	uri := r.schemaURI(eventType)

	// Compile schema
	doc, err := jschema.UnmarshalJSON(strings.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("jsonschema: parsing schema for %s: %w", eventType, err)
	}
	if err := r.compiler.AddResource(uri, doc); err != nil {
		return fmt.Errorf("jsonschema: adding resource for %s: %w", eventType, err)
	}
	compiled, err := r.compiler.Compile(uri)
	if err != nil {
		return fmt.Errorf("jsonschema: compiling schema for %s: %w", eventType, err)
	}

	// Store schema and track Go type
	r.mu.Lock()
	r.schemas[eventType] = &entry{compiled: compiled, raw: json.RawMessage(schemaJSON)}
	r.types[eventType] = t
	r.reverse[t] = eventType
	r.mu.Unlock()

	return nil
}

// MustRegisterType is like RegisterType but panics on error.
func (r *Registry) MustRegisterType(v any, schemaJSON string) {
	if err := r.RegisterType(v, schemaJSON); err != nil {
		panic(err)
	}
}

// Validate validates data against the schema for the given CloudEvents type.
// Returns nil if no schema is registered for the type (pass-through).
// Returns validation error if data doesn't conform to the schema.
func (r *Registry) Validate(eventType string, data []byte) error {
	r.mu.RLock()
	e, ok := r.schemas[eventType]
	r.mu.RUnlock()

	if !ok {
		return nil // No schema registered, pass through
	}

	inst, err := jschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return e.compiled.Validate(inst)
}

// Schema returns the raw JSON Schema for a CloudEvents type.
// Returns nil if no schema is registered for the type.
//
// Use this for serving individual schemas:
//
//	schema := registry.Schema("order.created")
//	w.Header().Set("Content-Type", "application/schema+json")
//	w.Write(schema)
func (r *Registry) Schema(eventType string) json.RawMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.schemas[eventType]; ok {
		return e.raw
	}
	return nil
}

// Schemas returns a composed JSON Schema document containing all registered
// schemas under $defs, keyed by CloudEvents type. The document is a valid
// JSON Schema that can be served as an API contract catalog.
//
// Use this for serving schema catalogs:
//
//	w.Header().Set("Content-Type", "application/schema+json")
//	w.Write(registry.Schemas())
func (r *Registry) Schemas() json.RawMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make(map[string]json.RawMessage, len(r.schemas))
	for eventType, e := range r.schemas {
		defs[eventType] = e.raw
	}

	doc := struct {
		Schema string                     `json:"$schema"`
		Defs   map[string]json.RawMessage `json:"$defs"`
	}{
		Schema: "https://json-schema.org/draft/2020-12/schema",
		Defs:   defs,
	}
	data, _ := json.Marshal(doc)
	return data
}

// NewInput creates a typed instance for the given CloudEvents type.
// Implements message.InputRegistry.
//
// Returns nil if no Go type was registered for the CloudEvents type.
func (r *Registry) NewInput(eventType string) any {
	r.mu.RLock()
	t, ok := r.types[eventType]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	return reflect.New(t).Interface()
}

// Verify Registry implements InputRegistry.
var _ message.InputRegistry = (*Registry)(nil)

func elemType(v any) reflect.Type {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func defaultSchemaURI(eventType string) string {
	return fmt.Sprintf("urn:gopipe:schema:cloudevents:%s", eventType)
}
