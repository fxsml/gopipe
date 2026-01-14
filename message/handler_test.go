package message

import (
	"context"
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
			KebabNaming,
		)

		if h.EventType() != "test.command" {
			t.Errorf("expected event type 'test.command', got %q", h.EventType())
		}
	})

	t.Run("NewInput creates typed instance", func(t *testing.T) {
		h := NewHandler[TestCommand](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			KebabNaming,
		)

		instance := h.NewInput()
		if _, ok := instance.(*TestCommand); !ok {
			t.Errorf("expected *TestCommand, got %T", instance)
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
			KebabNaming,
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
				Naming: KebabNaming,
			},
		)

		if h.EventType() != "test.command" {
			t.Errorf("expected event type 'test.command', got %q", h.EventType())
		}
	})

	t.Run("processes command and returns events", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return []TestEvent{{ID: cmd.ID, Status: "processed"}}, nil
			},
			CommandHandlerConfig{
				Source: "/test-service",
				Naming: KebabNaming,
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
		if out.SpecVersion() != "1.0" {
			t.Errorf("expected specversion '1.0', got %v", out.SpecVersion())
		}

		event, ok := out.Data.(TestEvent)
		if !ok {
			t.Fatalf("expected TestEvent, got %T", out.Data)
		}
		if event.ID != "abc" || event.Status != "processed" {
			t.Errorf("unexpected event: %+v", event)
		}
	})

	t.Run("returns error from handler", func(t *testing.T) {
		testErr := errors.New("handler error")
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				return nil, testErr
			},
			CommandHandlerConfig{
				Source: "/test",
				Naming: KebabNaming,
			},
		)

		msg := &Message{Data: &TestCommand{}}
		_, err := h.Handle(context.Background(), msg)
		if err != testErr {
			t.Errorf("expected error %v, got %v", testErr, err)
		}
	})

	t.Run("attributes available in context", func(t *testing.T) {
		var ctxAttrs Attributes
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
				ctxAttrs = AttributesFromContext(ctx)
				return nil, nil
			},
			CommandHandlerConfig{
				Source: "/test",
				Naming: KebabNaming,
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

		_, _ = h.Handle(context.Background(), msg)

		if ctxAttrs["id"] != "123" {
			t.Errorf("expected id '123' in context, got %v", ctxAttrs["id"])
		}
		if ctxAttrs["source"] != "/original" {
			t.Errorf("expected source '/original' in context, got %v", ctxAttrs["source"])
		}
	})
}

func TestNewUUID(t *testing.T) {
	t.Run("generates valid UUID v4 format", func(t *testing.T) {
		uuid := newUUID()
		// UUID format: 8-4-4-4-12
		if len(uuid) != 36 {
			t.Errorf("expected UUID length 36, got %d", len(uuid))
		}
		if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
			t.Errorf("invalid UUID format: %s", uuid)
		}
	})

	t.Run("generates unique UUIDs", func(t *testing.T) {
		uuids := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			uuid := newUUID()
			if uuids[uuid] {
				t.Errorf("duplicate UUID generated: %s", uuid)
			}
			uuids[uuid] = true
		}
	})
}
