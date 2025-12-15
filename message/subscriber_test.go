package message

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockReceiver is a test implementation of Receiver.
type mockReceiver struct {
	receiveFunc func(ctx context.Context, topic string) ([]*Message, error)
	callCount   map[string]int
	mu          sync.Mutex
}

func newMockReceiver() *mockReceiver {
	return &mockReceiver{
		callCount: make(map[string]int),
	}
}

func (m *mockReceiver) Receive(ctx context.Context, topic string) ([]*Message, error) {
	m.mu.Lock()
	m.callCount[topic]++
	m.mu.Unlock()

	if m.receiveFunc != nil {
		return m.receiveFunc(ctx, topic)
	}
	return nil, nil
}

func (m *mockReceiver) getCallCount(topic string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount[topic]
}

func TestSubscriber_Subscribe(t *testing.T) {
	ctx := context.Background()

	t.Run("single subscription", func(t *testing.T) {
		mock := newMockReceiver()
		callCount := 0
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			callCount++
			if callCount > 2 {
				// Block to stop further calls
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []*Message{
				New([]byte("msg1"), Attributes{}),
			}, nil
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{})

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		msgs := subscriber.Subscribe(ctx, "test.topic")

		var received []*Message
		for msg := range msgs {
			received = append(received, msg)
			if len(received) >= 2 {
				cancel() // Cancel to stop receiving more messages
				break
			}
		}

		if len(received) != 2 {
			t.Errorf("expected 2 messages, got %d", len(received))
		}
	})

	t.Run("multiple subscriptions to different topics", func(t *testing.T) {
		mock := newMockReceiver()
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			if mock.getCallCount(topic) > 1 {
				// Block to stop further calls for this topic
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []*Message{
				New([]byte(topic), Attributes{}),
			}, nil
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{})

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		msgs1 := subscriber.Subscribe(ctx, "topic1")
		msgs2 := subscriber.Subscribe(ctx, "topic2")

		// Read from first subscription
		msg1 := <-msgs1
		if string(msg1.Data) != "topic1" {
			t.Errorf("expected topic1, got %s", string(msg1.Data))
		}

		// Read from second subscription
		msg2 := <-msgs2
		if string(msg2.Data) != "topic2" {
			t.Errorf("expected topic2, got %s", string(msg2.Data))
		}

		// Both topics should have been called at least once
		if mock.getCallCount("topic1") < 1 {
			t.Errorf("expected at least 1 call to topic1, got %d", mock.getCallCount("topic1"))
		}
		if mock.getCallCount("topic2") < 1 {
			t.Errorf("expected at least 1 call to topic2, got %d", mock.getCallCount("topic2"))
		}
	})

	t.Run("multiple subscriptions to same topic", func(t *testing.T) {
		mock := newMockReceiver()
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			if mock.getCallCount(topic) > 4 {
				// Block to stop further calls
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []*Message{
				New([]byte("msg"), Attributes{}),
			}, nil
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{})

		ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// Subscribe to the same topic twice
		msgs1 := subscriber.Subscribe(ctx, "test.topic")
		msgs2 := subscriber.Subscribe(ctx, "test.topic")

		// Both subscriptions should independently receive messages
		msg1 := <-msgs1
		if msg1 == nil {
			t.Error("expected message from first subscription")
		}

		msg2 := <-msgs2
		if msg2 == nil {
			t.Error("expected message from second subscription")
		}

		// The receiver should have been called by both subscriptions
		// This demonstrates that each Subscribe call creates an independent subscription
		if mock.getCallCount("test.topic") < 1 {
			t.Errorf("expected at least 1 call to test.topic, got %d", mock.getCallCount("test.topic"))
		}
	})

	t.Run("subscription respects context cancellation", func(t *testing.T) {
		mock := newMockReceiver()
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return []*Message{
					New([]byte("msg"), Attributes{}),
				}, nil
			}
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{})

		ctx, cancel := context.WithCancel(ctx)
		msgs := subscriber.Subscribe(ctx, "test.topic")

		// Read one message
		msg := <-msgs
		if msg == nil {
			t.Error("expected message from subscription")
		}

		// Cancel context
		cancel()

		// Channel should close after context cancellation
		// Drain remaining messages until channel closes
		channelClosed := false
	drainLoop:
		for i := 0; i < 100; i++ {
			select {
			case _, ok := <-msgs:
				if !ok {
					channelClosed = true
					break drainLoop
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("channel did not close after context cancellation")
			}
		}

		if !channelClosed {
			t.Error("channel should be closed after context cancellation")
		}
	})

	t.Run("subscription with config options", func(t *testing.T) {
		mock := newMockReceiver()
		var callCount atomic.Int32
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			if callCount.Add(1) > 2 {
				return nil, errors.New("done")
			}
			return []*Message{
				New([]byte("msg"), Attributes{}),
			}, nil
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{
			Concurrency: 2,
			Timeout:     100 * time.Millisecond,
			Recover:     true,
		})

		ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		msgs := subscriber.Subscribe(ctx, "test.topic")

		var received []*Message
		for msg := range msgs {
			received = append(received, msg)
			if len(received) >= 2 {
				break
			}
		}

		if len(received) != 2 {
			t.Errorf("expected 2 messages, got %d", len(received))
		}
	})
}

func TestSubscriber_SubscribeMultipleCalls(t *testing.T) {
	t.Run("Subscribe can be called multiple times", func(t *testing.T) {
		mock := newMockReceiver()
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			if mock.getCallCount(topic) > 10 {
				// Block after many messages to allow all subscriptions to get at least one
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []*Message{
				New([]byte(topic), Attributes{}),
			}, nil
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Call Subscribe multiple times with different topics
		channels := []<-chan *Message{
			subscriber.Subscribe(ctx, "topic1"),
			subscriber.Subscribe(ctx, "topic2"),
			subscriber.Subscribe(ctx, "topic3"),
			subscriber.Subscribe(ctx, "topic1"), // Duplicate subscription to topic1
		}

		// Verify all channels are working
		for i, ch := range channels {
			msg := <-ch
			if msg == nil {
				t.Errorf("channel %d should receive a message", i)
			}
		}

		// Verify all topics were called
		if mock.getCallCount("topic1") < 1 {
			t.Errorf("expected at least 1 call to topic1, got %d", mock.getCallCount("topic1"))
		}
		if mock.getCallCount("topic2") < 1 {
			t.Errorf("expected at least 1 call to topic2, got %d", mock.getCallCount("topic2"))
		}
		if mock.getCallCount("topic3") < 1 {
			t.Errorf("expected at least 1 call to topic3, got %d", mock.getCallCount("topic3"))
		}
	})

	t.Run("Multiple subscriptions to same topic are independent", func(t *testing.T) {
		mock := newMockReceiver()
		mock.receiveFunc = func(ctx context.Context, topic string) ([]*Message, error) {
			if mock.getCallCount(topic) > 10 {
				// Block after some messages
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []*Message{
				New([]byte("msg"), Attributes{}),
			}, nil
		}

		subscriber := NewSubscriber(mock, SubscriberConfig{})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Create two independent subscriptions to the same topic
		sub1 := subscriber.Subscribe(ctx, "same.topic")
		sub2 := subscriber.Subscribe(ctx, "same.topic")

		// Each subscription should receive messages independently
		msg1 := <-sub1
		if msg1 == nil {
			t.Error("expected message from first subscription")
		}

		msg2 := <-sub2
		if msg2 == nil {
			t.Error("expected message from second subscription")
		}

		// Both subscriptions are active and independent
		// The behavior of receiving duplicate messages depends on the broker implementation
		if mock.getCallCount("same.topic") < 1 {
			t.Errorf("expected at least 1 call to same.topic, got %d", mock.getCallCount("same.topic"))
		}
	})
}
