package cloudevents

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/fxsml/gopipe/pipe"
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
}
