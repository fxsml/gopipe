package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestAutoAck_Success(t *testing.T) {
	var acked bool
	var nacked error
	acking := message.NewAcking(func() { acked = true }, func(err error) { nacked = err })

	msg := message.New("test", message.Attributes{"type": "test"}, acking)

	fn := AutoAck()(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return []*message.Message{message.New("result", nil, nil)}, nil
	})

	results, err := fn(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !acked {
		t.Error("expected message to be acked on success")
	}
	if nacked != nil {
		t.Errorf("unexpected nack: %v", nacked)
	}
}

func TestAutoAck_Error(t *testing.T) {
	var acked bool
	var nacked error
	acking := message.NewAcking(func() { acked = true }, func(err error) { nacked = err })

	msg := message.New("test", message.Attributes{"type": "test"}, acking)

	testErr := errors.New("processing failed")
	fn := AutoAck()(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return nil, testErr
	})

	results, err := fn(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected %v, got %v", testErr, err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
	if acked {
		t.Error("should not ack on error")
	}
	if !errors.Is(nacked, testErr) {
		t.Errorf("expected nack with %v, got %v", testErr, nacked)
	}
}

func TestAutoAck_NilAcking(t *testing.T) {
	// Message without acking should not panic
	msg := message.New("test", message.Attributes{"type": "test"}, nil)

	fn := AutoAck()(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return []*message.Message{message.New("result", nil, nil)}, nil
	})

	results, err := fn(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
