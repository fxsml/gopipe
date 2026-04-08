package cloudevents

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// mockReceiver implements protocol.Receiver for testing.
type mockReceiver struct {
	mu       sync.Mutex
	messages []binding.Message
	index    int
	err      error
}

func newMockReceiver(events ...*cloudevents.Event) *mockReceiver {
	var messages []binding.Message
	for _, e := range events {
		messages = append(messages, binding.ToMessage(e))
	}
	return &mockReceiver{messages: messages}
}

func (m *mockReceiver) Receive(ctx context.Context) (binding.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	if m.index >= len(m.messages) {
		return nil, io.EOF
	}

	msg := m.messages[m.index]
	m.index++
	return msg, nil
}

func TestSubscriber(t *testing.T) {
	t.Run("receives messages and bridges acking", func(t *testing.T) {
		event := cloudevents.NewEvent()
		event.SetID("test-id")
		event.SetType("test.type")
		event.SetSource("/test")
		if err := event.SetData("application/json", []byte(`{"key":"value"}`)); err != nil {
			t.Fatalf("failed to set data: %v", err)
		}

		receiver := newMockReceiver(&event)
		source := NewSubscriber(receiver, SubscriberConfig{Buffer: 10})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		ch, err := source.Subscribe(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case raw := <-ch:
			if raw.ID() != "test-id" {
				t.Errorf("expected id 'test-id', got %v", raw.ID())
			}
			if raw.Type() != "test.type" {
				t.Errorf("expected type 'test.type', got %v", raw.Type())
			}

			// Test acking
			if !raw.Ack() {
				t.Error("expected Ack to return true")
			}

		case <-ctx.Done():
			t.Fatal("timeout waiting for message")
		}
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		// Create a receiver that blocks forever
		receiver := &mockReceiver{}

		source := NewSubscriber(receiver, SubscriberConfig{})

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := source.Subscribe(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Cancel context - goroutine should exit and close channel
		cancel()

		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected channel to be closed after context cancellation")
			}
		case <-time.After(time.Second):
			t.Fatal("source did not stop on context cancellation")
		}
	})

	t.Run("closes channel on completion", func(t *testing.T) {
		receiver := newMockReceiver() // Empty receiver
		source := NewSubscriber(receiver, SubscriberConfig{})

		ch, err := source.Subscribe(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected channel to be closed")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for channel close")
		}
	})

	t.Run("returns error if called twice", func(t *testing.T) {
		receiver := newMockReceiver()
		source := NewSubscriber(receiver, SubscriberConfig{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := source.Subscribe(ctx)
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}

		_, err = source.Subscribe(ctx)
		if !errors.Is(err, pipe.ErrAlreadyStarted) {
			t.Errorf("expected ErrAlreadyStarted, got %v", err)
		}
	})

	t.Run("calls CleanupHandler on completion", func(t *testing.T) {
		receiver := newMockReceiver() // Empty receiver -> EOF

		cleaned := make(chan struct{})
		source := NewSubscriber(receiver, SubscriberConfig{
			CleanupHandler: func(context.Context) { close(cleaned) },
		})

		ch, err := source.Subscribe(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Drain the channel
		for range ch {
		}

		select {
		case <-cleaned:
			// success
		case <-time.After(time.Second):
			t.Error("expected Cleanup to be called")
		}
	})

	t.Run("calls CleanupHandler on context cancellation", func(t *testing.T) {
		// Create a receiver that blocks until context cancelled
		receiver := &mockReceiver{}

		cleaned := make(chan struct{})
		source := NewSubscriber(receiver, SubscriberConfig{
			CleanupHandler: func(context.Context) { close(cleaned) },
		})

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := source.Subscribe(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cancel()

		// Drain the channel
		for range ch {
		}

		select {
		case <-cleaned:
			// success
		case <-time.After(time.Second):
			t.Error("expected Cleanup to be called on context cancellation")
		}
	})

	t.Run("CleanupTimeout sets deadline on cleanup context", func(t *testing.T) {
		receiver := newMockReceiver() // Empty receiver -> EOF

		cleaned := make(chan struct{})
		source := NewSubscriber(receiver, SubscriberConfig{
			CleanupHandler: func(ctx context.Context) {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("expected context to have a deadline")
				}
				close(cleaned)
			},
			CleanupTimeout: 5 * time.Second,
		})

		ch, err := source.Subscribe(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for range ch {
		}

		select {
		case <-cleaned:
			// success
		case <-time.After(time.Second):
			t.Error("expected CleanupHandler to be called")
		}
	})

	t.Run("nil CleanupHandler does not panic", func(t *testing.T) {
		receiver := newMockReceiver() // Empty receiver -> EOF
		source := NewSubscriber(receiver, SubscriberConfig{
			CleanupHandler: nil,
		})

		ch, err := source.Subscribe(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected channel to be closed")
			}
			// success — no panic
		case <-time.After(time.Second):
			t.Error("expected channel to close")
		}
	})

	t.Run("Use applies middleware", func(t *testing.T) {
		event := cloudevents.NewEvent()
		event.SetID("test-id")
		event.SetType("test.type")
		event.SetSource("/test")
		if err := event.SetData("application/json", []byte(`{}`)); err != nil {
			t.Fatalf("failed to set data: %v", err)
		}

		receiver := newMockReceiver(&event)
		source := NewSubscriber(receiver, SubscriberConfig{Buffer: 10})

		var calls atomic.Int32
		countingMiddleware := func(next middleware.ProcessFunc[struct{}, *message.RawMessage]) middleware.ProcessFunc[struct{}, *message.RawMessage] {
			return func(ctx context.Context, in struct{}) ([]*message.RawMessage, error) {
				calls.Add(1)
				return next(ctx, in)
			}
		}

		if err := source.Use(countingMiddleware); err != nil {
			t.Fatalf("Use failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		ch, err := source.Subscribe(ctx)
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}

		<-ch // Wait for channel to close

		if calls.Load() == 0 {
			t.Error("middleware was not called")
		}
	})

	t.Run("Use returns error after Subscribe", func(t *testing.T) {
		receiver := newMockReceiver()
		source := NewSubscriber(receiver, SubscriberConfig{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, _ = source.Subscribe(ctx)

		err := source.Use(func(next middleware.ProcessFunc[struct{}, *message.RawMessage]) middleware.ProcessFunc[struct{}, *message.RawMessage] {
			return next
		})
		if !errors.Is(err, pipe.ErrAlreadyStarted) {
			t.Errorf("expected ErrAlreadyStarted, got %v", err)
		}
	})
}
