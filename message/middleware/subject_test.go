package middleware

import (
	"context"
	"testing"

	"github.com/fxsml/gopipe/message"
)

// orderCreated implements Subject() for testing.
type orderCreated struct {
	OrderID string
	Status  string
}

func (o orderCreated) Subject() string { return o.OrderID }

// simpleEvent does not implement Subject().
type simpleEvent struct {
	Data string
}

func TestSubject(t *testing.T) {
	t.Run("sets subject from Subjecter", func(t *testing.T) {
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.New(orderCreated{OrderID: "ORD-123", Status: "created"}, nil, nil),
			}, nil
		}

		wrapped := Subject()(handler)
		outputs, err := wrapped(context.Background(), message.New(nil, nil, nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}

		subject := outputs[0].Attributes[message.AttrSubject]
		if subject != "ORD-123" {
			t.Errorf("expected subject 'ORD-123', got %v", subject)
		}
	})

	t.Run("skips non-Subjecter data", func(t *testing.T) {
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.New(simpleEvent{Data: "test"}, nil, nil),
			}, nil
		}

		wrapped := Subject()(handler)
		outputs, err := wrapped(context.Background(), message.New(nil, nil, nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}

		if _, exists := outputs[0].Attributes[message.AttrSubject]; exists {
			t.Error("expected no subject attribute for non-Subjecter")
		}
	})

	t.Run("handles nil attributes", func(t *testing.T) {
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				{Data: orderCreated{OrderID: "ORD-456"}}, // nil Attributes
			}, nil
		}

		wrapped := Subject()(handler)
		outputs, err := wrapped(context.Background(), message.New(nil, nil, nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		subject := outputs[0].Attributes[message.AttrSubject]
		if subject != "ORD-456" {
			t.Errorf("expected subject 'ORD-456', got %v", subject)
		}
	})

	t.Run("handles multiple outputs", func(t *testing.T) {
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.New(orderCreated{OrderID: "ORD-1"}, nil, nil),
				message.New(simpleEvent{Data: "skip"}, nil, nil),
				message.New(orderCreated{OrderID: "ORD-2"}, nil, nil),
			}, nil
		}

		wrapped := Subject()(handler)
		outputs, err := wrapped(context.Background(), message.New(nil, nil, nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(outputs) != 3 {
			t.Fatalf("expected 3 outputs, got %d", len(outputs))
		}

		if outputs[0].Attributes[message.AttrSubject] != "ORD-1" {
			t.Errorf("expected first subject 'ORD-1', got %v", outputs[0].Attributes[message.AttrSubject])
		}
		if _, exists := outputs[1].Attributes[message.AttrSubject]; exists {
			t.Error("expected no subject on second output")
		}
		if outputs[2].Attributes[message.AttrSubject] != "ORD-2" {
			t.Errorf("expected third subject 'ORD-2', got %v", outputs[2].Attributes[message.AttrSubject])
		}
	})

	t.Run("propagates handler errors", func(t *testing.T) {
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return nil, message.ErrNoHandler
		}

		wrapped := Subject()(handler)
		_, err := wrapped(context.Background(), message.New(nil, nil, nil))
		if err != message.ErrNoHandler {
			t.Errorf("expected ErrNoHandler, got %v", err)
		}
	})
}
