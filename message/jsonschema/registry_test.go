package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/fxsml/gopipe/message"
)

const testSchema = `{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"type": "object",
	"properties": {
		"name":  { "type": "string", "minLength": 1 },
		"value": { "type": "number" }
	},
	"required": ["name", "value"],
	"additionalProperties": false
}`

type testData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

const testOtherSchema = `{
	"type": "object",
	"properties": {
		"id": { "type": "string" }
	},
	"required": ["id"]
}`

type testOther struct {
	ID string `json:"id"`
}

func TestRegistry_RegisterType(t *testing.T) {
	t.Run("registers type with KebabNaming", func(t *testing.T) {
		registry := NewRegistry(Config{})
		if err := registry.RegisterType(testData{}, testSchema); err != nil {
			t.Fatalf("RegisterType failed: %v", err)
		}

		// Should be accessible by derived eventType
		schema := registry.Schema("test.data")
		if schema == nil {
			t.Fatal("expected schema for test.data")
		}
	})

	t.Run("registers type with SnakeNaming", func(t *testing.T) {
		registry := NewRegistry(Config{Naming: message.SnakeNaming})
		registry.MustRegisterType(testData{}, testSchema)

		schema := registry.Schema("test_data")
		if schema == nil {
			t.Fatal("expected schema for test_data")
		}
	})

	t.Run("invalid schema JSON", func(t *testing.T) {
		registry := NewRegistry(Config{})
		err := registry.RegisterType(testData{}, `{not valid json}`)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("custom SchemaURI function", func(t *testing.T) {
		customURICalled := false
		registry := NewRegistry(Config{
			SchemaURI: func(eventType string) string {
				customURICalled = true
				return "https://example.com/schemas/" + eventType
			},
		})

		err := registry.RegisterType(testData{}, testSchema)
		if err != nil {
			t.Fatalf("RegisterType failed: %v", err)
		}

		if !customURICalled {
			t.Error("custom SchemaURI function was not called")
		}

		// Verify schema is registered and validates correctly
		if err := registry.Validate("test.data", []byte(`{"name":"ok","value":1}`)); err != nil {
			t.Errorf("validation failed with custom URI: %v", err)
		}
	})
}

func TestRegistry_Validate(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		err := registry.Validate("test.data", []byte(`{"name":"ok","value":42}`))
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		err := registry.Validate("test.data", []byte(`{"name":"ok"}`))
		if err == nil {
			t.Fatal("expected validation error for missing required field")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		err := registry.Validate("test.data", []byte(`{"name":"ok","value":"not-a-number"}`))
		if err == nil {
			t.Fatal("expected validation error for wrong type")
		}
	})

	t.Run("empty required string", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		err := registry.Validate("test.data", []byte(`{"name":"","value":1}`))
		if err == nil {
			t.Fatal("expected validation error for empty string with minLength: 1")
		}
	})

	t.Run("additional properties rejected", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		err := registry.Validate("test.data", []byte(`{"name":"ok","value":1,"extra":"field"}`))
		if err == nil {
			t.Fatal("expected validation error for additional properties")
		}
	})

	t.Run("no schema registered - pass through", func(t *testing.T) {
		registry := NewRegistry(Config{})

		err := registry.Validate("unknown.type", []byte(`{"any":"data"}`))
		if err != nil {
			t.Fatalf("expected nil for unregistered type, got: %v", err)
		}
	})
}

func TestRegistry_Schema(t *testing.T) {
	t.Run("returns schema by eventType", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		schema := registry.Schema("test.data")
		if schema == nil {
			t.Fatal("expected non-nil schema")
		}
		if !json.Valid(schema) {
			t.Error("schema is not valid JSON")
		}
	})

	t.Run("returns nil for unregistered type", func(t *testing.T) {
		registry := NewRegistry(Config{})
		schema := registry.Schema("unknown.type")
		if schema != nil {
			t.Errorf("expected nil, got %s", schema)
		}
	})
}

func TestRegistry_Schemas(t *testing.T) {
	t.Run("composes all registered types", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)
		registry.MustRegisterType(testOther{}, testOtherSchema)

		raw := registry.Schemas()
		if raw == nil {
			t.Fatal("expected non-nil schemas")
		}
		if !json.Valid(raw) {
			t.Fatal("schemas document is not valid JSON")
		}

		var doc struct {
			Schema string                     `json:"$schema"`
			Defs   map[string]json.RawMessage `json:"$defs"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if doc.Schema != "https://json-schema.org/draft/2020-12/schema" {
			t.Errorf("$schema = %q", doc.Schema)
		}
		if len(doc.Defs) != 2 {
			t.Fatalf("$defs count = %d, want 2", len(doc.Defs))
		}
		if _, ok := doc.Defs["test.data"]; !ok {
			t.Error("missing $defs/test.data")
		}
		if _, ok := doc.Defs["test.other"]; !ok {
			t.Error("missing $defs/test.other")
		}
	})

	t.Run("empty defs when no schemas", func(t *testing.T) {
		registry := NewRegistry(Config{})
		raw := registry.Schemas()

		var doc struct {
			Defs map[string]json.RawMessage `json:"$defs"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(doc.Defs) != 0 {
			t.Errorf("expected empty $defs, got %d", len(doc.Defs))
		}
	})
}

func TestRegistry_NewInput(t *testing.T) {
	t.Run("creates instance for registered type", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)

		instance := registry.NewInput("test.data")
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}

		ptr, ok := instance.(*testData)
		if !ok {
			t.Fatalf("expected *testData, got %T", instance)
		}

		if ptr.Name != "" || ptr.Value != 0 {
			t.Errorf("expected zero values, got Name=%q Value=%d", ptr.Name, ptr.Value)
		}
	})

	t.Run("returns nil for unregistered type", func(t *testing.T) {
		registry := NewRegistry(Config{})
		instance := registry.NewInput("unknown.type")
		if instance != nil {
			t.Errorf("expected nil, got %v", instance)
		}
	})

	t.Run("handles multiple registered types", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)
		registry.MustRegisterType(testOther{}, testOtherSchema)

		data := registry.NewInput("test.data")
		if data == nil {
			t.Error("expected testData instance")
		}

		other := registry.NewInput("test.other")
		if other == nil {
			t.Error("expected testOther instance")
		}

		unknown := registry.NewInput("test.unknown")
		if unknown != nil {
			t.Error("expected nil for unknown type")
		}
	})
}
