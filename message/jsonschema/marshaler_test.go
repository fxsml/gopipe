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

func TestMarshaler_Unmarshal(t *testing.T) {
	t.Run("no schema registered", func(t *testing.T) {
		m := NewMarshaler(Config{})

		var out testData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":1}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "ok" || out.Value != 1 {
			t.Errorf("got %+v, want {Name:ok Value:1}", out)
		}
	})

	t.Run("valid data", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		var out testData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":42}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "ok" || out.Value != 42 {
			t.Errorf("got %+v, want {Name:ok Value:42}", out)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"ok"}`), &out)
		if err == nil {
			t.Fatal("expected error for missing required field")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"ok","value":"not-a-number"}`), &out)
		if err == nil {
			t.Fatal("expected error for wrong type")
		}
	})

	t.Run("empty required string", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"","value":1}`), &out)
		if err == nil {
			t.Fatal("expected error for empty string with minLength: 1")
		}
	})

	t.Run("additional properties rejected", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"ok","value":1,"extra":"field"}`), &out)
		if err == nil {
			t.Fatal("expected error for additional properties")
		}
	})

	t.Run("does not decode when validation fails", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		out := testData{Name: "original"}
		_ = m.Unmarshal([]byte(`{"name":"changed"}`), &out)

		if out.Name != "original" {
			t.Errorf("Name = %q, want %q (unmarshal should not run after validation failure)", out.Name, "original")
		}
	})

	t.Run("pointer and value registration equivalent", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(&testData{}, testSchema) // register with pointer

		var out testData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":1}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMarshaler_Marshal(t *testing.T) {
	t.Run("no schema registered", func(t *testing.T) {
		m := NewMarshaler(Config{})

		data, err := m.Marshal(&testData{Name: "ok", Value: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != `{"name":"ok","value":1}` {
			t.Errorf("data = %s, want %s", data, `{"name":"ok","value":1}`)
		}
	})

	t.Run("valid data", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		data, err := m.Marshal(&testData{Name: "ok", Value: 7})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != `{"name":"ok","value":7}` {
			t.Errorf("data = %s, want %s", data, `{"name":"ok","value":7}`)
		}
	})

	t.Run("zero value fails required check", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		// Name is "" (zero value) — minLength:1 catches it.
		_, err := m.Marshal(&testData{Value: 1})
		if err == nil {
			t.Fatal("expected error for zero-value string with minLength: 1")
		}
	})
}

func TestMarshaler_DataContentType(t *testing.T) {
	m := NewMarshaler(Config{})
	if ct := m.DataContentType(); ct != "application/json" {
		t.Errorf("DataContentType() = %q, want %q", ct, "application/json")
	}
}

func TestMarshaler_Schema(t *testing.T) {
	t.Run("returns raw schema", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		raw := m.Schema(testData{})
		if raw == nil {
			t.Fatal("expected non-nil schema")
		}
		if !json.Valid(raw) {
			t.Error("schema is not valid JSON")
		}
	})

	t.Run("returns nil for unregistered type", func(t *testing.T) {
		m := NewMarshaler(Config{})
		if raw := m.Schema(testData{}); raw != nil {
			t.Errorf("expected nil, got %s", raw)
		}
	})
}

func TestMarshaler_Schemas(t *testing.T) {
	t.Run("composes all registered types", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)
		m.MustRegister(testOther{}, testOtherSchema)

		raw := m.Schemas()
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
		if _, ok := doc.Defs["testData"]; !ok {
			t.Error("missing $defs/testData")
		}
		if _, ok := doc.Defs["testOther"]; !ok {
			t.Error("missing $defs/testOther")
		}
	})

	t.Run("empty defs when no schemas", func(t *testing.T) {
		m := NewMarshaler(Config{})
		raw := m.Schemas()

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

	t.Run("each def is valid JSON Schema", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)

		raw := m.Schemas()
		var doc struct {
			Defs map[string]json.RawMessage `json:"$defs"`
		}
		json.Unmarshal(raw, &doc)

		for name, def := range doc.Defs {
			if !json.Valid(def) {
				t.Errorf("$defs/%s is not valid JSON", name)
			}
			var schema map[string]any
			if err := json.Unmarshal(def, &schema); err != nil {
				t.Errorf("$defs/%s: unmarshal failed: %v", name, err)
			}
			if _, ok := schema["type"]; !ok {
				t.Errorf("$defs/%s: missing 'type' keyword", name)
			}
		}
	})
}

func TestMarshaler_Register(t *testing.T) {
	t.Run("invalid schema JSON", func(t *testing.T) {
		m := NewMarshaler(Config{})
		err := m.Register(testData{}, `{not valid json}`)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestMarshaler_NewInput(t *testing.T) {
	t.Run("creates instance for registered type with KebabNaming", func(t *testing.T) {
		m := NewMarshaler(Config{}) // Default: KebabNaming
		m.MustRegister(testData{}, testSchema)

		// KebabNaming: testData → "test.data"
		instance := m.NewInput("test.data")
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}

		// Should be *testData
		ptr, ok := instance.(*testData)
		if !ok {
			t.Fatalf("expected *testData, got %T", instance)
		}

		// Should be zero-initialized
		if ptr.Name != "" || ptr.Value != 0 {
			t.Errorf("expected zero values, got Name=%q Value=%d", ptr.Name, ptr.Value)
		}
	})

	t.Run("creates instance with DefaultNaming", func(t *testing.T) {
		m := NewMarshaler(Config{Naming: message.DefaultNaming})
		m.MustRegister(testData{}, testSchema)

		// DefaultNaming: testData → "testData"
		instance := m.NewInput("testData")
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}

		_, ok := instance.(*testData)
		if !ok {
			t.Fatalf("expected *testData, got %T", instance)
		}
	})

	t.Run("creates instance with SnakeNaming", func(t *testing.T) {
		m := NewMarshaler(Config{Naming: message.SnakeNaming})
		m.MustRegister(testData{}, testSchema)

		// SnakeNaming: testData → "test_data"
		instance := m.NewInput("test_data")
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}

		_, ok := instance.(*testData)
		if !ok {
			t.Fatalf("expected *testData, got %T", instance)
		}
	})

	t.Run("returns nil for unregistered type", func(t *testing.T) {
		m := NewMarshaler(Config{})
		instance := m.NewInput("unknown.type")
		if instance != nil {
			t.Errorf("expected nil, got %v", instance)
		}
	})

	t.Run("handles multiple registered types", func(t *testing.T) {
		m := NewMarshaler(Config{})
		m.MustRegister(testData{}, testSchema)
		m.MustRegister(testOther{}, testOtherSchema)

		// Both types should be accessible
		data := m.NewInput("test.data")
		if data == nil {
			t.Error("expected testData instance")
		}

		other := m.NewInput("test.other")
		if other == nil {
			t.Error("expected testOther instance")
		}

		// Wrong type returns nil
		unknown := m.NewInput("test.unknown")
		if unknown != nil {
			t.Error("expected nil for unknown type")
		}
	})
}
