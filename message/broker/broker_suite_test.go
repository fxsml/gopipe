package broker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

// SenderFactory creates a Sender for testing.
type SenderFactory[T any] func(t *testing.T) broker.Sender[T]

// ReceiverFactory creates a Receiver for testing.
type ReceiverFactory[T any] func(t *testing.T) broker.Receiver[T]

// BrokerFactory creates a Broker for testing.
type BrokerFactory[T any] func(t *testing.T) broker.Broker[T]

// BrokerTestSuite runs a common set of tests against any broker implementation.
type BrokerTestSuite[T any] struct {
	// Name identifies the broker implementation being tested.
	Name string

	// NewBroker creates a new broker instance.
	NewBroker BrokerFactory[T]

	// SamplePayload returns a sample payload for testing.
	SamplePayload func() T

	// PayloadEquals compares two payloads for equality.
	PayloadEquals func(a, b T) bool

	// Skip lists test names to skip for this implementation.
	Skip map[string]string
}

// Run executes the test suite.
func (s *BrokerTestSuite[T]) Run(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, b broker.Broker[T])
	}{
		{"SendReceive", s.testSendReceive},
		{"MultipleMessages", s.testMultipleMessages},
		{"MultipleSubscribers", s.testMultipleSubscribers},
		{"MultipleTopics", s.testMultipleTopics},
		{"TopicsCreatedOnFly", s.testTopicsCreatedOnFly},
		{"ContextCancellation", s.testContextCancellation},
		{"HierarchicalTopics", s.testHierarchicalTopics},
	}

	for _, tt := range tests {
		t.Run(s.Name+"/"+tt.name, func(t *testing.T) {
			if reason, ok := s.Skip[tt.name]; ok {
				t.Skip(reason)
			}

			b := s.NewBroker(t)
			defer b.Close()

			tt.fn(t, b)
		})
	}
}

func (s *BrokerTestSuite[T]) testSendReceive(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start receiver before sending
	msgs := b.Receive(ctx, "test/topic")

	// Give receiver time to subscribe
	time.Sleep(20 * time.Millisecond)

	// Send message
	payload := s.SamplePayload()
	msg := message.New(payload, message.WithID[T]("msg-1"))
	if err := b.Send(ctx, "test/topic", msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive message
	select {
	case received := <-msgs:
		if !s.PayloadEquals(received.Payload(), payload) {
			t.Errorf("Payload mismatch")
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

func (s *BrokerTestSuite[T]) testMultipleMessages(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msgs := b.Receive(ctx, "batch")
	time.Sleep(20 * time.Millisecond)

	// Send multiple messages
	for i := 0; i < 3; i++ {
		payload := s.SamplePayload()
		if err := b.Send(ctx, "batch", message.New(payload)); err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	// Receive all messages
	received := 0
	timeout := time.After(time.Second)
	for received < 3 {
		select {
		case <-msgs:
			received++
		case <-timeout:
			t.Fatalf("Timeout after receiving %d messages", received)
		}
	}

	if received != 3 {
		t.Errorf("Expected 3 messages, got %d", received)
	}
}

func (s *BrokerTestSuite[T]) testMultipleSubscribers(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create multiple subscribers
	sub1 := b.Receive(ctx, "events")
	sub2 := b.Receive(ctx, "events")

	time.Sleep(20 * time.Millisecond)

	// Send message
	payload := s.SamplePayload()
	if err := b.Send(ctx, "events", message.New(payload)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Both subscribers should receive
	var wg sync.WaitGroup
	wg.Add(2)

	check := func(name string, ch <-chan *message.Message[T]) {
		defer wg.Done()
		select {
		case msg := <-ch:
			if !s.PayloadEquals(msg.Payload(), payload) {
				t.Errorf("%s: payload mismatch", name)
			}
		case <-ctx.Done():
			t.Errorf("%s: timeout", name)
		}
	}

	go check("sub1", sub1)
	go check("sub2", sub2)

	wg.Wait()
}

func (s *BrokerTestSuite[T]) testMultipleTopics(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	topic1 := b.Receive(ctx, "topic/one")
	topic2 := b.Receive(ctx, "topic/two")

	time.Sleep(20 * time.Millisecond)

	payload1 := s.SamplePayload()
	payload2 := s.SamplePayload()

	b.Send(ctx, "topic/one", message.New(payload1))
	b.Send(ctx, "topic/two", message.New(payload2))

	// Each topic should receive only its message
	select {
	case msg := <-topic1:
		if !s.PayloadEquals(msg.Payload(), payload1) {
			t.Error("topic1: wrong payload")
		}
	case <-ctx.Done():
		t.Fatal("topic1: timeout")
	}

	select {
	case msg := <-topic2:
		if !s.PayloadEquals(msg.Payload(), payload2) {
			t.Error("topic2: wrong payload")
		}
	case <-ctx.Done():
		t.Fatal("topic2: timeout")
	}
}

func (s *BrokerTestSuite[T]) testTopicsCreatedOnFly(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send to non-existent topic (should create it)
	payload := s.SamplePayload()
	if err := b.Send(ctx, "new/dynamic/topic", message.New(payload)); err != nil {
		t.Fatalf("Send to new topic failed: %v", err)
	}

	// Now subscribe and send again
	recv := b.Receive(ctx, "new/dynamic/topic")
	time.Sleep(20 * time.Millisecond)

	payload2 := s.SamplePayload()
	if err := b.Send(ctx, "new/dynamic/topic", message.New(payload2)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case msg := <-recv:
		if !s.PayloadEquals(msg.Payload(), payload2) {
			t.Error("payload mismatch")
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

func (s *BrokerTestSuite[T]) testContextCancellation(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithCancel(context.Background())
	recv := b.Receive(ctx, "test")

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-recv:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for channel close")
	}
}

func (s *BrokerTestSuite[T]) testHierarchicalTopics(t *testing.T, b broker.Broker[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	orders := b.Receive(ctx, "orders/created")
	users := b.Receive(ctx, "users/profile/updated")

	time.Sleep(20 * time.Millisecond)

	payload1 := s.SamplePayload()
	payload2 := s.SamplePayload()

	b.Send(ctx, "orders/created", message.New(payload1))
	b.Send(ctx, "users/profile/updated", message.New(payload2))

	select {
	case msg := <-orders:
		if !s.PayloadEquals(msg.Payload(), payload1) {
			t.Error("orders: wrong payload")
		}
	case <-ctx.Done():
		t.Fatal("orders: timeout")
	}

	select {
	case msg := <-users:
		if !s.PayloadEquals(msg.Payload(), payload2) {
			t.Error("users: wrong payload")
		}
	case <-ctx.Done():
		t.Fatal("users: timeout")
	}
}

// RunMemoryBrokerSuite runs the test suite against the in-memory broker.
func RunMemoryBrokerSuite(t *testing.T) {
	suite := &BrokerTestSuite[string]{
		Name: "MemoryBroker",
		NewBroker: func(t *testing.T) broker.Broker[string] {
			return broker.NewBroker[string](broker.Config{
				BufferSize: 100,
			})
		},
		SamplePayload: func() string {
			return "test-payload"
		},
		PayloadEquals: func(a, b string) bool {
			return a == b
		},
	}
	suite.Run(t)
}

func TestMemoryBrokerSuite(t *testing.T) {
	RunMemoryBrokerSuite(t)
}
