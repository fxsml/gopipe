package message

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type TestCommand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TestEvent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func TestNewHandler(t *testing.T) {
	t.Run("derives event type from Go type", func(t *testing.T) {
		h := NewHandler[TestCommand](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			DotNaming,
		)

		if h.EventType() != "test.command" {
			t.Errorf("expected event type 'test.command', got %q", h.EventType())
		}
	})

	t.Run("Handle processes typed message", func(t *testing.T) {
		var received TestCommand
		h := NewHandler[TestCommand](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				cmd := msg.Data.(*TestCommand)
				received = *cmd
				return []*Message{New(TestEvent{ID: cmd.ID, Status: "done"}, nil, nil)}, nil
			},
			DotNaming,
		)

		cmd := &TestCommand{ID: "123", Name: "test"}
		msg := &Message{
			Data:       cmd,
			Attributes: Attributes{"type": "test.command"},
		}

		outputs, err := h.Handle(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if received.ID != "123" || received.Name != "test" {
			t.Errorf("expected received command, got %+v", received)
		}
		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
	})
}

func TestNewCommandHandler(t *testing.T) {
	t.Run("derives event type from command type", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return nil, nil
			},
			CommandHandlerConfig{
				Source: "/test",
				Naming: DotNaming,
			},
		)

		if h.EventType() != "test.command" {
			t.Errorf("expected event type 'test.command', got %q", h.EventType())
		}
	})

	t.Run("unmarshals raw input and marshals output by default", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return []TestEvent{{ID: cmd.ID, Status: "processed"}}, nil
			},
			CommandHandlerConfig{
				Source: "/test-service",
				Naming: DotNaming,
			},
		)

		msg := NewRaw([]byte(`{"id":"abc","name":"test"}`), Attributes{"type": "test.command"}, nil)

		outputs, err := h.Handle(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}

		out := outputs[0]
		if out.Type() != "test.event" {
			t.Errorf("expected output type 'test.event', got %v", out.Type())
		}
		if out.Source() != "/test-service" {
			t.Errorf("expected source '/test-service', got %v", out.Source())
		}

		raw, ok := out.Raw()
		if !ok {
			t.Fatalf("expected raw output Data, got %T", out.Data)
		}
		var event TestEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("unexpected unmarshal error: %v", err)
		}
		if event.ID != "abc" || event.Status != "processed" {
			t.Errorf("unexpected event: %+v", event)
		}
		if out.Attributes[AttrDataContentType] != "application/json" {
			t.Errorf("expected datacontenttype 'application/json', got %v", out.Attributes[AttrDataContentType])
		}
	})

	t.Run("returns ErrUnexpectedDataType for non-raw input by default", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return nil, nil
			},
			CommandHandlerConfig{
				Source: "/test",
				Naming: DotNaming,
			},
		)

		msg := &Message{Data: TestCommand{ID: "123"}}
		_, err := h.Handle(context.Background(), msg)
		if !errors.Is(err, ErrUnexpectedDataType) {
			t.Fatalf("expected ErrUnexpectedDataType, got %v", err)
		}
	})

	t.Run("Subject sets CE subject attribute pre-marshal", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return []TestEvent{{ID: cmd.ID, Status: "processed"}}, nil
			},
			CommandHandlerConfig{
				Source: "/test",
				Naming: DotNaming,
				Subject: func(data any) string {
					return data.(TestEvent).ID
				},
			},
		)

		msg := NewRaw([]byte(`{"id":"xyz"}`), Attributes{"type": "test.command"}, nil)
		outputs, err := h.Handle(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if outputs[0].Subject() != "xyz" {
			t.Errorf("expected subject 'xyz', got %v", outputs[0].Subject())
		}
	})

	t.Run("DisableMarshaler operates typed-through", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return []TestEvent{{ID: cmd.ID, Status: "processed"}}, nil
			},
			CommandHandlerConfig{
				Source:           "/test-service",
				Naming:           DotNaming,
				DisableMarshaler: true,
			},
		)

		msg := &Message{
			Data:       &TestCommand{ID: "abc", Name: "test"},
			Attributes: Attributes{"type": "test.command"},
		}

		outputs, err := h.Handle(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}

		out := outputs[0]
		if out.Type() != "test.event" {
			t.Errorf("expected output type 'test.event', got %v", out.Type())
		}
		if out.Source() != "/test-service" {
			t.Errorf("expected source '/test-service', got %v", out.Source())
		}
		if _, ok := out.Attributes[AttrDataContentType]; ok {
			t.Errorf("expected no datacontenttype, got %v", out.Attributes[AttrDataContentType])
		}

		event, ok := out.Data.(TestEvent)
		if !ok {
			t.Fatalf("expected TestEvent, got %T", out.Data)
		}
		if event.ID != "abc" || event.Status != "processed" {
			t.Errorf("unexpected event: %+v", event)
		}
	})

	t.Run("DisableMarshaler returns error from handler", func(t *testing.T) {
		testErr := errors.New("handler error")
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return nil, testErr
			},
			CommandHandlerConfig{
				Source:           "/test",
				Naming:           DotNaming,
				DisableMarshaler: true,
			},
		)

		msg := &Message{Data: &TestCommand{}}
		_, err := h.Handle(context.Background(), msg)
		if err != testErr {
			t.Errorf("expected error %v, got %v", testErr, err)
		}
	})

	t.Run("DisableMarshaler returns error for nil data", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return nil, nil
			},
			CommandHandlerConfig{
				Source:           "/test",
				Naming:           DotNaming,
				DisableMarshaler: true,
			},
		)

		msg := &Message{Data: nil}
		_, err := h.Handle(context.Background(), msg)
		if !errors.Is(err, ErrCommandDataMismatch) {
			t.Fatalf("expected ErrCommandDataMismatch, got %v", err)
		}
	})

	t.Run("DisableMarshaler returns error for mismatched data type", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return nil, nil
			},
			CommandHandlerConfig{
				Source:           "/test",
				Naming:           DotNaming,
				DisableMarshaler: true,
			},
		)

		msg := &Message{Data: &TestEvent{ID: "wrong-type"}}
		_, err := h.Handle(context.Background(), msg)
		if !errors.Is(err, ErrCommandDataMismatch) {
			t.Fatalf("expected ErrCommandDataMismatch, got %v", err)
		}
	})

	t.Run("attributes available via message in context", func(t *testing.T) {
		var ctxMsg *Message
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				ctxMsg = MessageFromContext(ctx)
				return nil, nil
			},
			CommandHandlerConfig{
				Source:           "/test",
				Naming:           DotNaming,
				DisableMarshaler: true,
			},
		)

		msg := &Message{
			Data: &TestCommand{},
			Attributes: Attributes{
				"type":   "test.command",
				"source": "/original",
				"id":     "123",
			},
		}

		ctx := msg.Context(context.Background())
		_, _ = h.Handle(ctx, msg)

		if ctxMsg == nil {
			t.Fatal("expected message in context")
		}
		if ctxMsg.Attributes["id"] != "123" {
			t.Errorf("expected id '123' in context, got %v", ctxMsg.Attributes["id"])
		}
		if ctxMsg.Attributes["source"] != "/original" {
			t.Errorf("expected source '/original' in context, got %v", ctxMsg.Attributes["source"])
		}
	})

	t.Run("config attributes merged into outputs", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
			},
			CommandHandlerConfig{
				Source:           "/test",
				Naming:           DotNaming,
				DisableMarshaler: true,
				Attributes: Attributes{
					AttrDataSchema: "https://example.com/schema.json",
					"customext":    "custom-value",
				},
			},
		)

		msg := &Message{Data: &TestCommand{ID: "123"}}
		outputs, err := h.Handle(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := outputs[0]
		if out.DataSchema() != "https://example.com/schema.json" {
			t.Errorf("expected dataschema, got %v", out.DataSchema())
		}
		if out.Attributes["customext"] != "custom-value" {
			t.Errorf("expected customext 'custom-value', got %v", out.Attributes["customext"])
		}
		// Core attributes should still be set
		if out.Source() != "/test" {
			t.Errorf("expected source '/test', got %v", out.Source())
		}
		if out.ID() == "" {
			t.Error("expected id to be set")
		}
	})
}
