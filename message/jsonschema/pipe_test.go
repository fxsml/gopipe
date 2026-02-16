package jsonschema

import (
	"context"
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestUnmarshalPipe(t *testing.T) {
	t.Run("unmarshals and validates", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		pipe := NewUnmarshalPipe(m, message.PipeConfig{})

		in := make(chan *message.RawMessage, 1)
		out, err := pipe.Pipe(context.Background(), in)
		if err != nil {
			t.Fatalf("pipe failed: %v", err)
		}

		// Send valid data with correct CloudEvents type
		in <- message.NewRaw(
			[]byte(`{"name":"test","value":42}`),
			message.Attributes{
				message.AttrSpecVersion: "1.0",
				message.AttrType:        "test.data", // KebabNaming: testData → "test.data"
				message.AttrSource:      "/test",
				message.AttrID:          "1",
			},
			nil,
		)
		close(in)

		// Should unmarshal successfully
		msg := <-out
		if msg == nil {
			t.Fatal("expected message, got nil")
		}

		data, ok := msg.Data.(*testData)
		if !ok {
			t.Fatalf("expected *testData, got %T", msg.Data)
		}
		if data.Name != "test" || data.Value != 42 {
			t.Errorf("got Name=%q Value=%d, want Name=%q Value=%d",
				data.Name, data.Value, "test", 42)
		}
	})

	t.Run("rejects invalid data", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		pipe := NewUnmarshalPipe(m, message.PipeConfig{})

		in := make(chan *message.RawMessage, 1)
		out, err := pipe.Pipe(context.Background(), in)
		if err != nil {
			t.Fatalf("pipe failed: %v", err)
		}

		// Send invalid data (missing required field)
		raw := message.NewRaw(
			[]byte(`{"name":"test"}`), // missing "value"
			message.Attributes{
				message.AttrSpecVersion: "1.0",
				message.AttrType:        "test.data",
				message.AttrSource:      "/test",
				message.AttrID:          "1",
			},
			nil,
		)
		in <- raw
		close(in)

		// Should be rejected (nacked), nothing comes out
		select {
		case msg := <-out:
			if msg != nil {
				t.Errorf("expected no message, got %v", msg)
			}
		default:
			// Channel closed without message - validation rejected it
		}
	})

	t.Run("returns error for unknown type", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		pipe := NewUnmarshalPipe(m, message.PipeConfig{})

		in := make(chan *message.RawMessage, 1)
		out, err := pipe.Pipe(context.Background(), in)
		if err != nil {
			t.Fatalf("pipe failed: %v", err)
		}

		// Send message with unknown type
		raw := message.NewRaw(
			[]byte(`{"name":"test","value":42}`),
			message.Attributes{
				message.AttrSpecVersion: "1.0",
				message.AttrType:        "unknown.type",
				message.AttrSource:      "/test",
				message.AttrID:          "1",
			},
			nil,
		)
		in <- raw
		close(in)

		// Should be rejected (no instance created)
		select {
		case msg := <-out:
			if msg != nil {
				t.Errorf("expected no message for unknown type, got %v", msg)
			}
		default:
			// Channel closed without message
		}
	})
}

func TestMarshalPipe(t *testing.T) {
	t.Run("marshals and validates", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		pipe := NewMarshalPipe(m, message.PipeConfig{})

		in := make(chan *message.Message, 1)
		out, err := pipe.Pipe(context.Background(), in)
		if err != nil {
			t.Fatalf("pipe failed: %v", err)
		}

		// Send valid message
		in <- &message.Message{
			Data: &testData{Name: "test", Value: 42},
			Attributes: message.Attributes{
				message.AttrSpecVersion: "1.0",
				message.AttrType:        "test.data",
				message.AttrSource:      "/test",
				message.AttrID:          "1",
			},
		}
		close(in)

		// Should marshal successfully
		raw := <-out
		if raw == nil {
			t.Fatal("expected raw message, got nil")
		}

		// Verify JSON
		expected := `{"name":"test","value":42}`
		if string(raw.Data) != expected {
			t.Errorf("got %s, want %s", raw.Data, expected)
		}
	})

	t.Run("rejects invalid data", func(t *testing.T) {
		m := NewMarshaler()
		m.MustRegister(testData{}, testSchema)

		pipe := NewMarshalPipe(m, message.PipeConfig{})

		in := make(chan *message.Message, 1)
		out, err := pipe.Pipe(context.Background(), in)
		if err != nil {
			t.Fatalf("pipe failed: %v", err)
		}

		// Send invalid message (zero value fails minLength: 1)
		msg := &message.Message{
			Data: &testData{Name: "", Value: 42}, // Empty name fails minLength
			Attributes: message.Attributes{
				message.AttrSpecVersion: "1.0",
				message.AttrType:        "test.data",
				message.AttrSource:      "/test",
				message.AttrID:          "1",
			},
		}
		in <- msg
		close(in)

		// Should be rejected (nacked), nothing comes out
		select {
		case raw := <-out:
			if raw != nil {
				t.Errorf("expected no message, got %v", raw)
			}
		default:
			// Channel closed without message - validation rejected it
		}
	})

	t.Run("passes through unregistered types", func(t *testing.T) {
		m := NewMarshaler()
		// Don't register testOther

		pipe := NewMarshalPipe(m, message.PipeConfig{})

		in := make(chan *message.Message, 1)
		out, err := pipe.Pipe(context.Background(), in)
		if err != nil {
			t.Fatalf("pipe failed: %v", err)
		}

		// Send unregistered type (should pass through without validation)
		in <- &message.Message{
			Data: &testOther{ID: "123"},
			Attributes: message.Attributes{
				message.AttrSpecVersion: "1.0",
				message.AttrType:        "test.other",
				message.AttrSource:      "/test",
				message.AttrID:          "1",
			},
		}
		close(in)

		// Should marshal without validation
		raw := <-out
		if raw == nil {
			t.Fatal("expected raw message, got nil")
		}

		expected := `{"id":"123"}`
		if string(raw.Data) != expected {
			t.Errorf("got %s, want %s", raw.Data, expected)
		}
	})
}
