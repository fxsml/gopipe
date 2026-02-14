package message

import (
	"errors"
	"testing"
)

var errValidation = errors.New("validation failed")

func TestValidatingMarshaler_Unmarshal(t *testing.T) {
	t.Run("passes when no validator set", func(t *testing.T) {
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{})

		var out PipeTestData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":1}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "ok" || out.Value != 1 {
			t.Errorf("got %+v, want {Name:ok Value:1}", out)
		}
	})

	t.Run("rejects invalid data", func(t *testing.T) {
		validator := func(data []byte, v any) error {
			return errValidation
		}
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{
			UnmarshalValidation: validator,
		})

		var out PipeTestData
		err := m.Unmarshal([]byte(`{"name":"ok"}`), &out)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errValidation) {
			t.Errorf("error = %v, want wrapping %v", err, errValidation)
		}
	})

	t.Run("does not unmarshal when validation fails", func(t *testing.T) {
		validator := func(data []byte, v any) error {
			return errValidation
		}
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{
			UnmarshalValidation: validator,
		})

		out := PipeTestData{Name: "original"}
		_ = m.Unmarshal([]byte(`{"name":"changed"}`), &out)

		if out.Name != "original" {
			t.Errorf("Name = %q, want %q (unmarshal should not run after validation failure)", out.Name, "original")
		}
	})

	t.Run("accepts valid data", func(t *testing.T) {
		validator := func(data []byte, v any) error {
			return nil
		}
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{
			UnmarshalValidation: validator,
		})

		var out PipeTestData
		if err := m.Unmarshal([]byte(`{"name":"ok","value":5}`), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "ok" || out.Value != 5 {
			t.Errorf("got %+v, want {Name:ok Value:5}", out)
		}
	})
}

func TestValidatingMarshaler_Marshal(t *testing.T) {
	t.Run("passes when no validator set", func(t *testing.T) {
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{})

		data, err := m.Marshal(&PipeTestData{Name: "ok", Value: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != `{"name":"ok","value":1}` {
			t.Errorf("data = %s, want %s", data, `{"name":"ok","value":1}`)
		}
	})

	t.Run("rejects invalid output", func(t *testing.T) {
		validator := func(data []byte, v any) error {
			return errValidation
		}
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{
			MarshalValidation: validator,
		})

		_, err := m.Marshal(&PipeTestData{Name: "test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errValidation) {
			t.Errorf("error = %v, want wrapping %v", err, errValidation)
		}
	})

	t.Run("accepts valid output", func(t *testing.T) {
		validator := func(data []byte, v any) error {
			return nil
		}
		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{
			MarshalValidation: validator,
		})

		data, err := m.Marshal(&PipeTestData{Name: "ok", Value: 7})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != `{"name":"ok","value":7}` {
			t.Errorf("data = %s, want %s", data, `{"name":"ok","value":7}`)
		}
	})

	t.Run("propagates inner marshal error", func(t *testing.T) {
		m := NewValidatingMarshaler(&failingMarshaler{}, ValidatingMarshalerConfig{
			MarshalValidation: func(data []byte, v any) error {
				t.Error("validator should not be called when inner marshal fails")
				return nil
			},
		})

		_, err := m.Marshal(&PipeTestData{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestValidatingMarshaler_DataContentType(t *testing.T) {
	m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{})

	if ct := m.DataContentType(); ct != "application/json" {
		t.Errorf("DataContentType() = %q, want %q", ct, "application/json")
	}
}

func TestValidatingMarshaler_BothDirections(t *testing.T) {
	t.Run("validates both marshal and unmarshal", func(t *testing.T) {
		var unmarshalCalled, marshalCalled bool

		m := NewValidatingMarshaler(NewJSONMarshaler(), ValidatingMarshalerConfig{
			UnmarshalValidation: func(data []byte, v any) error {
				unmarshalCalled = true
				return nil
			},
			MarshalValidation: func(data []byte, v any) error {
				marshalCalled = true
				return nil
			},
		})

		// Marshal
		data, err := m.Marshal(&PipeTestData{Name: "test", Value: 1})
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !marshalCalled {
			t.Error("marshal validator was not called")
		}

		// Unmarshal
		var out PipeTestData
		if err := m.Unmarshal(data, &out); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if !unmarshalCalled {
			t.Error("unmarshal validator was not called")
		}
	})
}
