package message

import (
	"context"
	"testing"
	"time"
)

type PipeTestData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestNewUnmarshalPipe(t *testing.T) {
	t.Run("unmarshals known type", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()

		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *RawMessage, 1)
		in <- &RawMessage{
			Data:       []byte(`{"name":"test","value":42}`),
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, err := p.Pipe(ctx, in)
		if err != nil {
			t.Fatalf("Pipe() error = %v", err)
		}

		msg := <-out
		if msg == nil {
			t.Fatal("expected message, got nil")
		}

		data, ok := msg.Data.(*PipeTestData)
		if !ok {
			t.Fatalf("Data type = %T, want *PipeTestData", msg.Data)
		}

		if data.Name != "test" || data.Value != 42 {
			t.Errorf("Data = %+v, want {Name:test Value:42}", data)
		}
	})

	t.Run("returns error for unknown type", func(t *testing.T) {
		registry := FactoryMap{}
		marshaler := NewJSONMarshaler()

		var lastErr error
		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{
			ErrorHandler: func(msg *Message, err error) {
				lastErr = err
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *RawMessage, 1)
		in <- &RawMessage{
			Data:       []byte(`{}`),
			Attributes: Attributes{"type": "unknown.type"},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)

		// Drain output
		for range out {
		}

		if lastErr != ErrUnknownType {
			t.Errorf("error = %v, want %v", lastErr, ErrUnknownType)
		}
	})

	t.Run("auto-nacks on error", func(t *testing.T) {
		registry := FactoryMap{}
		marshaler := NewJSONMarshaler()

		var nacked bool
		acking := NewAcking(func() {}, func(err error) { nacked = true })

		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *RawMessage, 1)
		in <- &RawMessage{
			Data:       []byte(`{}`),
			Attributes: Attributes{"type": "unknown.type"},
			acking:     acking,
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		for range out {
		}

		if !nacked {
			t.Error("expected message to be auto-nacked on error")
		}
	})

	t.Run("preserves attributes", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()

		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *RawMessage, 1)
		in <- &RawMessage{
			Data: []byte(`{}`),
			Attributes: Attributes{
				"type":   "test.data",
				"source": "/test",
				"id":     "123",
			},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		msg := <-out

		if msg.Source() != "/test" {
			t.Errorf("source = %v, want /test", msg.Source())
		}
		if msg.ID() != "123" {
			t.Errorf("id = %v, want 123", msg.ID())
		}
	})
}

func TestNewMarshalPipe(t *testing.T) {
	t.Run("marshals data to JSON", func(t *testing.T) {
		marshaler := NewJSONMarshaler()

		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       &PipeTestData{Name: "test", Value: 42},
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, err := p.Pipe(ctx, in)
		if err != nil {
			t.Fatalf("Pipe() error = %v", err)
		}

		raw := <-out
		if raw == nil {
			t.Fatal("expected message, got nil")
		}

		expected := `{"name":"test","value":42}`
		if string(raw.Data) != expected {
			t.Errorf("Data = %s, want %s", raw.Data, expected)
		}
	})

	t.Run("auto-nacks on error", func(t *testing.T) {
		// Use a marshaler that will fail
		marshaler := &failingMarshaler{}

		var nacked bool
		acking := NewAcking(func() {}, func(err error) { nacked = true })

		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       &PipeTestData{},
			Attributes: Attributes{"type": "test.data"},
			acking:     acking,
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		for range out {
		}

		if !nacked {
			t.Error("expected message to be auto-nacked on marshal error")
		}
	})

	t.Run("sets datacontenttype", func(t *testing.T) {
		marshaler := NewJSONMarshaler()

		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       &PipeTestData{},
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		raw := <-out

		if raw.DataContentType() != "application/json" {
			t.Errorf("datacontenttype = %v, want application/json", raw.DataContentType())
		}
	})

	t.Run("creates attributes if nil", func(t *testing.T) {
		marshaler := NewJSONMarshaler()

		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       &PipeTestData{},
			Attributes: nil,
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		raw := <-out

		if raw.Attributes == nil {
			t.Fatal("Attributes = nil, want non-nil")
		}
		if raw.DataContentType() != "application/json" {
			t.Errorf("datacontenttype = %v, want application/json", raw.DataContentType())
		}
	})

	t.Run("preserves attributes", func(t *testing.T) {
		marshaler := NewJSONMarshaler()

		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data: &PipeTestData{},
			Attributes: Attributes{
				"type":   "test.data",
				"source": "/test",
				"id":     "123",
			},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		raw := <-out

		if raw.Source() != "/test" {
			t.Errorf("source = %v, want /test", raw.Source())
		}
		if raw.ID() != "123" {
			t.Errorf("id = %v, want 123", raw.ID())
		}
	})
}

func TestPipeRoundtrip(t *testing.T) {
	t.Run("unmarshal and marshal roundtrip", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()

		unmarshal := NewUnmarshalPipe(registry, marshaler, PipeConfig{})
		marshal := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		// Start with raw message
		rawIn := make(chan *RawMessage, 1)
		rawIn <- &RawMessage{
			Data:       []byte(`{"name":"roundtrip","value":99}`),
			Attributes: Attributes{"type": "test.data", "source": "/test"},
		}
		close(rawIn)

		// Unmarshal
		typed, _ := unmarshal.Pipe(ctx, rawIn)

		// Marshal
		rawOut, _ := marshal.Pipe(ctx, typed)

		// Verify result
		result := <-rawOut

		expected := `{"name":"roundtrip","value":99}`
		if string(result.Data) != expected {
			t.Errorf("Data = %s, want %s", result.Data, expected)
		}
		if result.Type() != "test.data" {
			t.Errorf("type = %v, want test.data", result.Type())
		}
		if result.Source() != "/test" {
			t.Errorf("source = %v, want /test", result.Source())
		}
		if result.DataContentType() != "application/json" {
			t.Errorf("datacontenttype = %v, want application/json", result.DataContentType())
		}
	})
}

func TestPipeConfigDefaults(t *testing.T) {
	t.Run("pool defaults", func(t *testing.T) {
		cfg := PipeConfig{}.parse()
		if cfg.Pool.Workers != 1 {
			t.Errorf("expected Workers=1, got %d", cfg.Pool.Workers)
		}
		if cfg.Pool.BufferSize != 100 {
			t.Errorf("expected BufferSize=100, got %d", cfg.Pool.BufferSize)
		}
		if cfg.Logger == nil {
			t.Error("expected default Logger")
		}
	})
}

// failingMarshaler always fails to marshal
type failingMarshaler struct{}

func (m *failingMarshaler) Marshal(v any) ([]byte, error) {
	return nil, ErrUnknownType // Reuse existing error
}

func (m *failingMarshaler) Unmarshal(data []byte, v any) error {
	return ErrUnknownType
}

func (m *failingMarshaler) DataContentType() string {
	return "application/json"
}
