package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
	"github.com/fxsml/gopipe/pubsub/memory"
)

// BrokerFactory creates a Broker for testing.
type BrokerFactory func(t *testing.T) pubsub.Broker

// BrokerTestSuite runs a common set of tests against any broker implementation.
type BrokerTestSuite struct {
	// Name identifies the broker implementation being tested.
	Name string

	// NewBroker creates a new broker instance.
	NewBroker BrokerFactory

	// SamplePayload returns a sample payload for testing.
	SamplePayload func() []byte

	// PayloadEquals compares two payloads for equality.
	PayloadEquals func(a, b []byte) bool

	// Skip lists test names to skip for this implementation.
	Skip map[string]string
}

// Run executes the test suite.
func (s *BrokerTestSuite) Run(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, b pubsub.Broker)
	}{
		{"SendReceive", s.testSendReceive},
		{"MultipleMessages", s.testMultipleMessages},
		{"MultipleTopics", s.testMultipleTopics},
		{"TopicsCreatedOnFly", s.testTopicsCreatedOnFly},
		{"HierarchicalTopics", s.testHierarchicalTopics},
	}

	for _, tt := range tests {
		t.Run(s.Name+"/"+tt.name, func(t *testing.T) {
			if reason, ok := s.Skip[tt.name]; ok {
				t.Skip(reason)
			}

			b := s.NewBroker(t)

			tt.fn(t, b)
		})
	}
}

func (s *BrokerTestSuite) testSendReceive(t *testing.T, b pubsub.Broker) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send message
	payload := s.SamplePayload()
	msg := message.New(payload, message.Properties{"id": "msg-1"})
	if err := b.Send(ctx, "test/topic", []*message.Message{msg}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive message
	msgs, err := b.Receive(ctx, "test/topic")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("No message received")
	}
	if !s.PayloadEquals(msgs[0].Payload, payload) {
		t.Errorf("Payload mismatch")
	}
}

func (s *BrokerTestSuite) testMultipleMessages(t *testing.T, b pubsub.Broker) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send multiple messages
	msgs := []*message.Message{
		message.New(s.SamplePayload(), message.Properties{}),
		message.New(s.SamplePayload(), message.Properties{}),
		message.New(s.SamplePayload(), message.Properties{}),
	}
	if err := b.Send(ctx, "batch", msgs); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive all messages
	received, err := b.Receive(ctx, "batch")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(received) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(received))
	}
}

// testMultipleSubscribers is skipped as the new API doesn't support channel-based pub/sub

func (s *BrokerTestSuite) testMultipleTopics(t *testing.T, b pubsub.Broker) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	payload1 := s.SamplePayload()
	payload2 := s.SamplePayload()

	b.Send(ctx, "topic/one", []*message.Message{message.New(payload1, message.Properties{})})
	b.Send(ctx, "topic/two", []*message.Message{message.New(payload2, message.Properties{})})

	// Each topic should receive only its message
	msgs1, err1 := b.Receive(ctx, "topic/one")
	if err1 != nil || len(msgs1) == 0 {
		t.Fatal("topic1: failed to receive")
	}
	if !s.PayloadEquals(msgs1[0].Payload, payload1) {
		t.Error("topic1: wrong payload")
	}

	msgs2, err2 := b.Receive(ctx, "topic/two")
	if err2 != nil || len(msgs2) == 0 {
		t.Fatal("topic2: failed to receive")
	}
	if !s.PayloadEquals(msgs2[0].Payload, payload2) {
		t.Error("topic2: wrong payload")
	}
}

func (s *BrokerTestSuite) testTopicsCreatedOnFly(t *testing.T, b pubsub.Broker) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send to non-existent topic (should create it)
	payload := s.SamplePayload()
	if err := b.Send(ctx, "new/dynamic/topic", []*message.Message{message.New(payload, message.Properties{})}); err != nil {
		t.Fatalf("Send to new topic failed: %v", err)
	}

	// Send again
	payload2 := s.SamplePayload()
	if err := b.Send(ctx, "new/dynamic/topic", []*message.Message{message.New(payload2, message.Properties{})}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive
	msgs, err := b.Receive(ctx, "new/dynamic/topic")
	if err != nil || len(msgs) == 0 {
		t.Fatal("Receive failed")
	}
	// Should have at least one message
	found := false
	for _, msg := range msgs {
		if s.PayloadEquals(msg.Payload, payload2) {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected payload not found")
	}
}

// testContextCancellation is skipped as new API doesn't use channels

func (s *BrokerTestSuite) testHierarchicalTopics(t *testing.T, b pubsub.Broker) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	payload1 := s.SamplePayload()
	payload2 := s.SamplePayload()

	b.Send(ctx, "orders/created", []*message.Message{message.New(payload1, message.Properties{})})
	b.Send(ctx, "users/profile/updated", []*message.Message{message.New(payload2, message.Properties{})})

	orders, err1 := b.Receive(ctx, "orders/created")
	if err1 != nil || len(orders) == 0 {
		t.Fatal("orders: failed to receive")
	}
	if !s.PayloadEquals(orders[0].Payload, payload1) {
		t.Error("orders: wrong payload")
	}

	users, err2 := b.Receive(ctx, "users/profile/updated")
	if err2 != nil || len(users) == 0 {
		t.Fatal("users: failed to receive")
	}
	if !s.PayloadEquals(users[0].Payload, payload2) {
		t.Error("users: wrong payload")
	}
}

// RunMemoryBrokerSuite runs the test suite against the in-memory broker.
func RunMemoryBrokerSuite(t *testing.T) {
	suite := &BrokerTestSuite{
		Name: "MemoryBroker",
		NewBroker: func(t *testing.T) pubsub.Broker {
			return memory.NewBroker(memory.Config{
				BufferSize: 100,
			})
		},
		SamplePayload: func() []byte {
			return []byte("test-payload")
		},
		PayloadEquals: func(a, b []byte) bool {
			return string(a) == string(b)
		},
	}
	suite.Run(t)
}

func TestMemoryBrokerSuite(t *testing.T) {
	RunMemoryBrokerSuite(t)
}
