package redis

import (
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestDefaultMarshalRoundTrip(t *testing.T) {
	original := message.NewRaw(
		[]byte(`{"order_id":"123"}`),
		message.Attributes{
			message.AttrID:     "evt-1",
			message.AttrType:   "order.created",
			message.AttrSource: "/orders",
		},
		nil,
	)

	values, err := DefaultMarshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if values[fieldData] != `{"order_id":"123"}` {
		t.Fatalf("unexpected data: %v", values[fieldData])
	}

	got, err := DefaultUnmarshal("1-0", values)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if string(got.Data) != string(original.Data) {
		t.Fatalf("data mismatch: got %s, want %s", got.Data, original.Data)
	}
	if got.ID() != original.ID() {
		t.Fatalf("ID mismatch: got %s, want %s", got.ID(), original.ID())
	}
	if got.Type() != original.Type() {
		t.Fatalf("Type mismatch: got %s, want %s", got.Type(), original.Type())
	}
	if got.Source() != original.Source() {
		t.Fatalf("Source mismatch: got %s, want %s", got.Source(), original.Source())
	}
}

func TestDefaultUnmarshalErrors(t *testing.T) {
	t.Run("missing data field", func(t *testing.T) {
		_, err := DefaultUnmarshal("1-0", map[string]any{
			fieldAttributes: `{}`,
		})
		if err == nil {
			t.Fatal("expected error for missing data field")
		}
	})

	t.Run("invalid attributes JSON", func(t *testing.T) {
		_, err := DefaultUnmarshal("1-0", map[string]any{
			fieldData:       `{}`,
			fieldAttributes: `not json`,
		})
		if err == nil {
			t.Fatal("expected error for invalid attributes")
		}
	})

	t.Run("missing attributes is ok", func(t *testing.T) {
		got, err := DefaultUnmarshal("1-0", map[string]any{
			fieldData: `{"ok":true}`,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got.Data) != `{"ok":true}` {
			t.Fatalf("unexpected data: %s", got.Data)
		}
	})
}
