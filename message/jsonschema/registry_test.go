package jsonschema

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	jschema "github.com/santhosh-tekuri/jsonschema/v6"
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

	t.Run("no schema registered returns error", func(t *testing.T) {
		registry := NewRegistry(Config{})

		err := registry.Validate("unknown.type", []byte(`{"any":"data"}`))
		if err == nil {
			t.Fatal("expected error for unregistered type")
		}
		if !strings.Contains(err.Error(), "no schema registered") {
			t.Errorf("expected 'no schema registered' error, got: %v", err)
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

	t.Run("composed document is valid JSON Schema", func(t *testing.T) {
		registry := NewRegistry(Config{})
		registry.MustRegisterType(testData{}, testSchema)
		registry.MustRegisterType(testOther{}, testOtherSchema)

		raw := registry.Schemas()

		compiler := jschema.NewCompiler()
		doc, err := jschema.UnmarshalJSON(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatalf("unmarshal as JSON Schema: %v", err)
		}
		if err := compiler.AddResource("urn:test:catalog", doc); err != nil {
			t.Fatalf("add resource: %v", err)
		}
		if _, err := compiler.Compile("urn:test:catalog"); err != nil {
			t.Fatalf("composed schema does not compile as valid JSON Schema: %v", err)
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

func TestRegistry_MustRegisterType_panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid schema")
		}
	}()
	registry := NewRegistry(Config{})
	registry.MustRegisterType(testData{}, `{not valid}`)
}

func TestRegistry_RegisterType_pointer(t *testing.T) {
	registry := NewRegistry(Config{})
	if err := registry.RegisterType(&testData{}, testSchema); err != nil {
		t.Fatalf("RegisterType with pointer failed: %v", err)
	}
	if schema := registry.Schema("test.data"); schema == nil {
		t.Fatal("expected schema for test.data via pointer registration")
	}
}

func TestRegistry_Validate_invalid_json(t *testing.T) {
	registry := NewRegistry(Config{})
	registry.MustRegisterType(testData{}, testSchema)

	err := registry.Validate("test.data", []byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewValidationMiddleware(t *testing.T) {
	registry := NewRegistry(Config{})
	registry.MustRegisterType(testData{}, testSchema)
	mw := NewValidationMiddleware(registry)

	passthrough := func(_ context.Context, raw *message.RawMessage) ([]*message.RawMessage, error) {
		return []*message.RawMessage{raw}, nil
	}
	fn := mw(passthrough)

	t.Run("valid message passes through", func(t *testing.T) {
		raw := message.NewRaw([]byte(`{"name":"ok","value":1}`), message.Attributes{"type": "test.data"}, nil)
		results, err := fn(context.Background(), raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("invalid message rejected", func(t *testing.T) {
		raw := message.NewRaw([]byte(`{"name":"ok"}`), message.Attributes{"type": "test.data"}, nil)
		_, err := fn(context.Background(), raw)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "validation failed") {
			t.Errorf("expected 'validation failed' prefix, got: %v", err)
		}
	})

	t.Run("unregistered type returns error", func(t *testing.T) {
		raw := message.NewRaw([]byte(`{"any":"data"}`), message.Attributes{"type": "unknown"}, nil)
		_, err := fn(context.Background(), raw)
		if err == nil {
			t.Fatal("expected error for unregistered type")
		}
	})
}

func TestNewInputValidationMiddleware(t *testing.T) {
	registry := NewRegistry(Config{})
	registry.MustRegisterType(testData{}, testSchema)
	mw := NewInputValidationMiddleware(registry)

	next := func(_ context.Context, raw *message.RawMessage) ([]*message.Message, error) {
		return []*message.Message{message.New(nil, raw.Attributes, nil)}, nil
	}
	fn := mw(next)

	t.Run("valid message passes to next", func(t *testing.T) {
		raw := message.NewRaw([]byte(`{"name":"ok","value":1}`), message.Attributes{"type": "test.data"}, nil)
		results, err := fn(context.Background(), raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("invalid message rejected before next", func(t *testing.T) {
		raw := message.NewRaw([]byte(`{"name":"ok"}`), message.Attributes{"type": "test.data"}, nil)
		_, err := fn(context.Background(), raw)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "input validation failed") {
			t.Errorf("expected 'input validation failed' prefix, got: %v", err)
		}
	})
}

func TestNewOutputValidationMiddleware(t *testing.T) {
	registry := NewRegistry(Config{})
	registry.MustRegisterType(testData{}, testSchema)
	mw := NewOutputValidationMiddleware(registry)

	t.Run("valid output passes", func(t *testing.T) {
		next := func(_ context.Context, msg *message.Message) ([]*message.RawMessage, error) {
			return []*message.RawMessage{
				message.NewRaw([]byte(`{"name":"ok","value":1}`), message.Attributes{"type": "test.data"}, nil),
			}, nil
		}
		fn := mw(next)

		msg := message.New(testData{Name: "ok", Value: 1}, message.Attributes{"type": "test.data"}, nil)
		results, err := fn(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("invalid output rejected", func(t *testing.T) {
		next := func(_ context.Context, msg *message.Message) ([]*message.RawMessage, error) {
			return []*message.RawMessage{
				message.NewRaw([]byte(`{"name":"ok"}`), message.Attributes{"type": "test.data"}, nil),
			}, nil
		}
		fn := mw(next)

		msg := message.New(nil, message.Attributes{"type": "test.data"}, nil)
		_, err := fn(context.Background(), msg)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "output validation failed") {
			t.Errorf("expected 'output validation failed' prefix, got: %v", err)
		}
	})

	t.Run("next error propagated", func(t *testing.T) {
		next := func(_ context.Context, msg *message.Message) ([]*message.RawMessage, error) {
			return nil, context.Canceled
		}
		fn := mw(next)

		msg := message.New(nil, message.Attributes{"type": "test.data"}, nil)
		_, err := fn(context.Background(), msg)
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	})
}
