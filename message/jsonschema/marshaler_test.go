package jsonschema

import (
	"encoding/json"
	"testing"
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

func TestMarshaler_Unmarshal(t *testing.T) {
	t.Run("no schema registered", func(t *testing.T) {
		m := NewMarshaler()

		var out testData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":1}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "ok" || out.Value != 1 {
			t.Errorf("got %+v, want {Name:ok Value:1}", out)
		}
	})

	t.Run("valid data", func(t *testing.T) {
		m := NewMarshaler()
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
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"ok"}`), &out)
		if err == nil {
			t.Fatal("expected error for missing required field")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"ok","value":"not-a-number"}`), &out)
		if err == nil {
			t.Fatal("expected error for wrong type")
		}
	})

	t.Run("empty required string", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"","value":1}`), &out)
		if err == nil {
			t.Fatal("expected error for empty string with minLength: 1")
		}
	})

	t.Run("additional properties rejected", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		var out testData
		err := m.Unmarshal([]byte(`{"name":"ok","value":1,"extra":"field"}`), &out)
		if err == nil {
			t.Fatal("expected error for additional properties")
		}
	})

	t.Run("does not decode when validation fails", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		out := testData{Name: "original"}
		_ = m.Unmarshal([]byte(`{"name":"changed"}`), &out)

		if out.Name != "original" {
			t.Errorf("Name = %q, want %q (unmarshal should not run after validation failure)", out.Name, "original")
		}
	})

	t.Run("pointer and value registration equivalent", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(&testData{}, testSchema) // register with pointer

		var out testData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":1}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMarshaler_Marshal(t *testing.T) {
	t.Run("no schema registered", func(t *testing.T) {
		m := NewMarshaler()

		data, err := m.Marshal(&testData{Name: "ok", Value: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != `{"name":"ok","value":1}` {
			t.Errorf("data = %s, want %s", data, `{"name":"ok","value":1}`)
		}
	})

	t.Run("valid data", func(t *testing.T) {
		m := NewMarshaler()
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
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		// Name is "" (zero value) — minLength:1 catches it.
		_, err := m.Marshal(&testData{Value: 1})
		if err == nil {
			t.Fatal("expected error for zero-value string with minLength: 1")
		}
	})
}

func TestMarshaler_DataContentType(t *testing.T) {
	m := NewMarshaler()
	if ct := m.DataContentType(); ct != "application/json" {
		t.Errorf("DataContentType() = %q, want %q", ct, "application/json")
	}
}

func TestMarshaler_Schema(t *testing.T) {
	t.Run("returns raw schema", func(t *testing.T) {
		m := NewMarshaler()
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
		m := NewMarshaler()
		if raw := m.Schema(testData{}); raw != nil {
			t.Errorf("expected nil, got %s", raw)
		}
	})
}

func TestMarshaler_Register(t *testing.T) {
	t.Run("invalid schema JSON", func(t *testing.T) {
		m := NewMarshaler()
		err := m.Register(testData{}, `{not valid json}`)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}
