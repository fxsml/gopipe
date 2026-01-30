package http

import (
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestParseBatchBytes(t *testing.T) {
	t.Run("parses valid batch", func(t *testing.T) {
		data := []byte(`[
			{"specversion":"1.0","id":"1","type":"test.type","source":"/test","data":{"key":"value1"}},
			{"specversion":"1.0","id":"2","type":"test.type","source":"/test","data":{"key":"value2"}}
		]`)

		msgs, err := ParseBatchBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}

		if msgs[0].ID() != "1" {
			t.Errorf("expected id '1', got %v", msgs[0].ID())
		}
		if msgs[1].ID() != "2" {
			t.Errorf("expected id '2', got %v", msgs[1].ID())
		}
	})

	t.Run("parses empty batch", func(t *testing.T) {
		data := []byte(`[]`)

		msgs, err := ParseBatchBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(msgs) != 0 {
			t.Fatalf("expected 0 messages, got %d", len(msgs))
		}
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		data := []byte(`not json`)

		_, err := ParseBatchBytes(data)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("returns error on invalid event in batch", func(t *testing.T) {
		data := []byte(`[{"specversion":"1.0"},{"invalid]`)

		_, err := ParseBatchBytes(data)
		if err == nil {
			t.Fatal("expected error for invalid event")
		}
	})
}

func TestMarshalBatch(t *testing.T) {
	t.Run("marshals messages to batch", func(t *testing.T) {
		msgs := []*message.RawMessage{
			message.NewRaw([]byte(`{"key":"value1"}`), message.Attributes{
				message.AttrID:          "1",
				message.AttrType:        "test.type",
				message.AttrSource:      "/test",
				message.AttrSpecVersion: "1.0",
			}, nil),
			message.NewRaw([]byte(`{"key":"value2"}`), message.Attributes{
				message.AttrID:          "2",
				message.AttrType:        "test.type",
				message.AttrSource:      "/test",
				message.AttrSpecVersion: "1.0",
			}, nil),
		}

		data, err := MarshalBatch(msgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify it's valid batch JSON by parsing it back
		parsed, err := ParseBatchBytes(data)
		if err != nil {
			t.Fatalf("failed to parse marshaled batch: %v", err)
		}

		if len(parsed) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(parsed))
		}

		if parsed[0].ID() != "1" {
			t.Errorf("expected id '1', got %v", parsed[0].ID())
		}
	})

	t.Run("marshals empty batch", func(t *testing.T) {
		data, err := MarshalBatch([]*message.RawMessage{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(data) != "[]" {
			t.Errorf("expected '[]', got %s", string(data))
		}
	})
}
