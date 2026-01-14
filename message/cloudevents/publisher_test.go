package cloudevents

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/fxsml/gopipe/message"
)

// mockSender implements protocol.Sender for testing.
type mockSender struct {
	mu       sync.Mutex
	messages []binding.Message
	results  []protocol.Result
	index    int
}

func newMockSender(results ...protocol.Result) *mockSender {
	return &mockSender{results: results}
}

func (m *mockSender) Send(ctx context.Context, msg binding.Message, transformers ...binding.Transformer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = append(m.messages, msg)

	if m.index < len(m.results) {
		result := m.results[m.index]
		m.index++
		return result
	}

	return protocol.ResultACK
}

func (m *mockSender) MessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func TestPublisher(t *testing.T) {
	t.Run("sends messages and acks on success", func(t *testing.T) {
		sender := newMockSender(protocol.ResultACK)
		pub := NewPublisher(sender, PublisherConfig{})

		in := make(chan *message.RawMessage, 1)

		acked := make(chan bool, 1)
		raw := message.NewRaw(
			[]byte(`{"key":"value"}`),
			message.Attributes{
				"id":     "test-id",
				"type":   "test.type",
				"source": "/test",
			},
			message.NewAcking(
				func() { acked <- true },
				func(err error) { t.Errorf("unexpected nack: %v", err) },
			),
		)
		in <- raw
		close(in)

		done, err := pub.Publish(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case <-acked:
			// success
		case <-time.After(time.Second):
			t.Error("expected message to be acked")
		}

		// Wait for done
		select {
		case <-done:
			// success
		case <-time.After(time.Second):
			t.Error("expected done channel to close")
		}

		if sender.MessageCount() != 1 {
			t.Errorf("expected 1 message sent, got %d", sender.MessageCount())
		}
	})

	t.Run("nacks on send failure", func(t *testing.T) {
		sender := newMockSender(protocol.NewReceipt(false, "send failed"))
		pub := NewPublisher(sender, PublisherConfig{})

		in := make(chan *message.RawMessage, 1)

		nacked := make(chan bool, 1)
		raw := message.NewRaw(
			[]byte(`{"key":"value"}`),
			message.Attributes{
				"id":     "test-id",
				"type":   "test.type",
				"source": "/test",
			},
			message.NewAcking(
				func() { t.Error("unexpected ack") },
				func(err error) { nacked <- true },
			),
		)
		in <- raw
		close(in)

		done, err := pub.Publish(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case <-nacked:
			// success
		case <-time.After(time.Second):
			t.Error("expected message to be nacked")
		}

		<-done // Wait for completion
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		sender := newMockSender()
		pub := NewPublisher(sender, PublisherConfig{})

		in := make(chan *message.RawMessage) // Blocking channel

		ctx, cancel := context.WithCancel(context.Background())
		done, err := pub.Publish(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Cancel context and close input - both needed to unblock workers
		cancel()
		close(in)

		select {
		case <-done:
			// success
		case <-time.After(time.Second):
			t.Error("expected done channel to close after context cancellation")
		}
	})

	t.Run("exits when channel closes", func(t *testing.T) {
		sender := newMockSender()
		pub := NewPublisher(sender, PublisherConfig{})

		in := make(chan *message.RawMessage)
		close(in)

		done, err := pub.Publish(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case <-done:
			// success
		case <-time.After(time.Second):
			t.Error("expected done channel to close")
		}
	})

	t.Run("returns error if called twice", func(t *testing.T) {
		sender := newMockSender()
		pub := NewPublisher(sender, PublisherConfig{})

		in := make(chan *message.RawMessage)
		close(in)

		_, err := pub.Publish(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}

		_, err = pub.Publish(context.Background(), in)
		if err == nil {
			t.Error("expected error on second call")
		}
	})
}
