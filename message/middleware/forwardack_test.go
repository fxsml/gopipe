package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestForwardAck(t *testing.T) {
	t.Run("acks input when all outputs acked", func(t *testing.T) {
		var inputAcked bool
		acking := message.NewAcking(func() { inputAcked = true }, func(error) {})
		msg := message.New("test", message.Attributes{"type": "test"}, acking)

		handler := func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.New("out1", nil, nil),
				message.New("out2", nil, nil),
			}, nil
		}

		wrapped := ForwardAck()(handler)
		outputs, err := wrapped(context.Background(), msg)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(outputs) != 2 {
			t.Fatalf("expected 2 outputs, got %d", len(outputs))
		}

		// Input should not be acked yet
		if inputAcked {
			t.Error("input should not be acked before outputs")
		}

		// Ack first output
		outputs[0].Ack()
		if inputAcked {
			t.Error("input should not be acked after only 1 output")
		}

		// Ack second output - now input should be acked
		outputs[1].Ack()
		if !inputAcked {
			t.Error("input should be acked after all outputs")
		}
	})

	t.Run("nacks input when any output nacked", func(t *testing.T) {
		var inputNacked bool
		var nackErr error
		acking := message.NewAcking(func() {}, func(err error) {
			inputNacked = true
			nackErr = err
		})
		msg := message.New("test", message.Attributes{"type": "test"}, acking)

		handler := func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.New("out1", nil, nil),
				message.New("out2", nil, nil),
			}, nil
		}

		wrapped := ForwardAck()(handler)
		outputs, _ := wrapped(context.Background(), msg)

		// Nack first output
		testErr := errors.New("processing failed")
		outputs[0].Nack(testErr)

		if !inputNacked {
			t.Error("input should be nacked when any output nacks")
		}
		if nackErr != testErr {
			t.Errorf("expected error %v, got %v", testErr, nackErr)
		}

		// Subsequent ack should be no-op
		outputs[1].Ack()
		// No additional assertions needed - just verify no panic
	})

	t.Run("acks input immediately when no outputs", func(t *testing.T) {
		var inputAcked bool
		acking := message.NewAcking(func() { inputAcked = true }, func(error) {})
		msg := message.New("test", message.Attributes{"type": "test"}, acking)

		handler := func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			return nil, nil
		}

		wrapped := ForwardAck()(handler)
		outputs, err := wrapped(context.Background(), msg)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(outputs) != 0 {
			t.Fatalf("expected 0 outputs, got %d", len(outputs))
		}
		if !inputAcked {
			t.Error("input should be acked when handler returns no outputs")
		}
	})

	t.Run("nacks input on handler error", func(t *testing.T) {
		var inputNacked bool
		acking := message.NewAcking(func() {}, func(error) { inputNacked = true })
		msg := message.New("test", message.Attributes{"type": "test"}, acking)

		handlerErr := errors.New("handler failed")
		handler := func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			return nil, handlerErr
		}

		wrapped := ForwardAck()(handler)
		_, err := wrapped(context.Background(), msg)

		if err != handlerErr {
			t.Errorf("expected error %v, got %v", handlerErr, err)
		}
		if !inputNacked {
			t.Error("input should be nacked on handler error")
		}
	})

	t.Run("skips forwarding if input already acked", func(t *testing.T) {
		var inputAcked int
		acking := message.NewAcking(func() { inputAcked++ }, func(error) {})
		msg := message.New("test", message.Attributes{"type": "test"}, acking)

		handler := func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			// Handler acks manually
			m.Ack()
			return []*message.Message{
				message.New("out1", nil, nil),
			}, nil
		}

		wrapped := ForwardAck()(handler)
		outputs, _ := wrapped(context.Background(), msg)

		// Input was already acked by handler
		if inputAcked != 1 {
			t.Errorf("expected input acked once, got %d", inputAcked)
		}

		// Output acking should be nil (not set up)
		if outputs[0].Acking != nil {
			t.Error("output should have nil acking when input already acked")
		}
	})

	t.Run("context cancelled on nack", func(t *testing.T) {
		acking := message.NewAcking(func() {}, func(error) {})
		msg := message.New("test", message.Attributes{"type": "test"}, acking)

		handler := func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.New("out1", nil, nil),
				message.New("out2", nil, nil),
			}, nil
		}

		wrapped := ForwardAck()(handler)
		outputs, _ := wrapped(context.Background(), msg)

		// Get context before nack
		ctx1 := outputs[0].Context()
		ctx2 := outputs[1].Context()

		// Contexts should be the same (shared acking)
		if ctx1 != ctx2 {
			t.Error("outputs should share the same context")
		}

		// Context should not be cancelled yet
		if ctx1.Err() != nil {
			t.Error("context should not be cancelled before nack")
		}

		// Nack one output
		outputs[0].Nack(errors.New("fail"))

		// Now context should be cancelled
		if ctx1.Err() == nil {
			t.Error("context should be cancelled after nack")
		}
	})
}
