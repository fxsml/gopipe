package message

import (
	"context"
	"errors"
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

		in := make(chan *Message, 1)
		in <- &Message{
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

	t.Run("preserves locals across unmarshal", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()

		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		type keyType struct{}
		raw := &Message{
			Data:       []byte(`{"name":"test","value":1}`),
			Attributes: Attributes{"type": "test.data"},
		}
		raw.SetLocal(keyType{}, "principal-123")

		in := make(chan *Message, 1)
		in <- raw
		close(in)

		out, err := p.Pipe(ctx, in)
		if err != nil {
			t.Fatalf("Pipe() error = %v", err)
		}

		msg := <-out
		if msg == nil {
			t.Fatal("expected message, got nil")
		}

		got := msg.Local(keyType{})
		if got != "principal-123" {
			t.Errorf("Local() = %v, want principal-123", got)
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

		in := make(chan *Message, 1)
		in <- &Message{
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

	t.Run("returns ErrDataNotRaw when Data is not []byte", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()

		var lastErr error
		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{
			ErrorHandler: func(msg *Message, err error) {
				lastErr = err
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       &PipeTestData{Name: "already-typed"},
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		for range out {
		}

		if !errors.Is(lastErr, ErrDataNotRaw) {
			t.Errorf("error = %v, want %v", lastErr, ErrDataNotRaw)
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

		in := make(chan *Message, 1)
		in <- &Message{
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

		in := make(chan *Message, 1)
		in <- &Message{
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

		data, ok := raw.Raw()
		if !ok {
			t.Fatalf("Data type = %T, want []byte", raw.Data)
		}

		expected := `{"name":"test","value":42}`
		if string(data) != expected {
			t.Errorf("Data = %s, want %s", data, expected)
		}
	})

	t.Run("returns ErrDataNotTyped when Data is already raw", func(t *testing.T) {
		marshaler := NewJSONMarshaler()

		var lastErr error
		p := NewMarshalPipe(marshaler, PipeConfig{
			ErrorHandler: func(msg *Message, err error) {
				lastErr = err
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       []byte(`{"already":"raw"}`),
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		for range out {
		}

		if !errors.Is(lastErr, ErrDataNotTyped) {
			t.Errorf("error = %v, want %v", lastErr, ErrDataNotTyped)
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

	t.Run("preserves locals across marshal", func(t *testing.T) {
		marshaler := NewJSONMarshaler()

		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		type keyType struct{}
		msg := &Message{
			Data:       &PipeTestData{Name: "test", Value: 1},
			Attributes: Attributes{"type": "test.data"},
		}
		msg.SetLocal(keyType{}, "tenant-abc")

		in := make(chan *Message, 1)
		in <- msg
		close(in)

		out, _ := p.Pipe(ctx, in)
		raw := <-out

		got := raw.Local(keyType{})
		if got != "tenant-abc" {
			t.Errorf("Local() = %v, want tenant-abc", got)
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
		rawIn := make(chan *Message, 1)
		rawIn <- &Message{
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

		data, ok := result.Raw()
		if !ok {
			t.Fatalf("Data type = %T, want []byte", result.Data)
		}

		expected := `{"name":"roundtrip","value":99}`
		if string(data) != expected {
			t.Errorf("Data = %s, want %s", data, expected)
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

func TestUnmarshalPipe_Use(t *testing.T) {
	t.Run("adds middleware before start", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()
		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{})

		// Add middleware that modifies value
		mw := func(next ProcessFunc) ProcessFunc {
			return func(ctx context.Context, in *Message) ([]*Message, error) {
				msgs, err := next(ctx, in)
				if err == nil && len(msgs) > 0 {
					data := msgs[0].Data.(*PipeTestData)
					data.Value = data.Value * 2
				}
				return msgs, err
			}
		}

		err := p.Use(mw)
		if err != nil {
			t.Fatalf("Use() error = %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       []byte(`{"name":"test","value":21}`),
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		msg := <-out

		data := msg.Data.(*PipeTestData)
		if data.Value != 42 {
			t.Errorf("middleware not applied: Value = %d, want 42", data.Value)
		}
	})

	t.Run("returns error after pipe started", func(t *testing.T) {
		registry := FactoryMap{
			"test.data": func() any { return &PipeTestData{} },
		}
		marshaler := NewJSONMarshaler()
		p := NewUnmarshalPipe(registry, marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message)
		close(in)

		_, _ = p.Pipe(ctx, in)

		// Try to add middleware after start
		err := p.Use(func(next ProcessFunc) ProcessFunc {
			return next
		})

		if err != ErrAlreadyStarted {
			t.Errorf("Use() after start error = %v, want %v", err, ErrAlreadyStarted)
		}
	})
}

func TestMarshalPipe_Use(t *testing.T) {
	t.Run("adds middleware before start", func(t *testing.T) {
		marshaler := NewJSONMarshaler()
		p := NewMarshalPipe(marshaler, PipeConfig{})

		// Add middleware that modifies value
		mw := func(next ProcessFunc) ProcessFunc {
			return func(ctx context.Context, in *Message) ([]*Message, error) {
				data := in.Data.(*PipeTestData)
				data.Value = data.Value * 2
				return next(ctx, in)
			}
		}

		err := p.Use(mw)
		if err != nil {
			t.Fatalf("Use() error = %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message, 1)
		in <- &Message{
			Data:       &PipeTestData{Name: "test", Value: 21},
			Attributes: Attributes{"type": "test.data"},
		}
		close(in)

		out, _ := p.Pipe(ctx, in)
		raw := <-out

		data, ok := raw.Raw()
		if !ok {
			t.Fatalf("Data type = %T, want []byte", raw.Data)
		}

		expected := `{"name":"test","value":42}`
		if string(data) != expected {
			t.Errorf("middleware not applied: Data = %s, want %s", data, expected)
		}
	})

	t.Run("returns error after pipe started", func(t *testing.T) {
		marshaler := NewJSONMarshaler()
		p := NewMarshalPipe(marshaler, PipeConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		in := make(chan *Message)
		close(in)

		_, _ = p.Pipe(ctx, in)

		// Try to add middleware after start
		err := p.Use(func(next ProcessFunc) ProcessFunc {
			return next
		})

		if err != ErrAlreadyStarted {
			t.Errorf("Use() after start error = %v, want %v", err, ErrAlreadyStarted)
		}
	})
}
