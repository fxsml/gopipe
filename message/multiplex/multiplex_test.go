package multiplex_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
	"github.com/fxsml/gopipe/message/multiplex"
)

// ============================================================================
// Mock implementations
// ============================================================================

type mockSender struct {
	mu      sync.Mutex
	sent    map[string][][]*message.Message
	sendErr error
}

func newMockSender() *mockSender {
	return &mockSender{
		sent: make(map[string][][]*message.Message),
	}
}

func (m *mockSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent[topic] = append(m.sent[topic], msgs)
	return nil
}

func (m *mockSender) getSent(topic string) [][]*message.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[topic]
}

type mockReceiver struct {
	mu       sync.Mutex
	messages map[string][]*message.Message
}

func newMockReceiver() *mockReceiver {
	return &mockReceiver{
		messages: make(map[string][]*message.Message),
	}
}

func (m *mockReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.messages[topic]
	m.messages[topic] = nil
	return msgs, nil
}

func (m *mockReceiver) addMessages(topic string, msgs ...*message.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[topic] = append(m.messages[topic], msgs...)
}

// ============================================================================
// Sender Tests
// ============================================================================

func TestSender_Basic(t *testing.T) {
	memorySender := newMockSender()
	fallbackSender := newMockSender()

	// Use "/" separator for prefix matching
	selector := multiplex.PrefixSenderSelector("internal", memorySender)
	mux := multiplex.NewSender(selector, fallbackSender)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	tests := []struct {
		topic          string
		expectedSender *mockSender
	}{
		{"internal/events", memorySender},
		{"internal/cache", memorySender},
		{"external/api", fallbackSender},
		{"orders/created", fallbackSender},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			// Clear previous sends
			memorySender.sent = make(map[string][][]*message.Message)
			fallbackSender.sent = make(map[string][][]*message.Message)

			err := mux.Send(ctx, tt.topic, []*message.Message{msg})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check which sender received the message
			memorySent := len(memorySender.getSent(tt.topic))
			fallbackSent := len(fallbackSender.getSent(tt.topic))

			if tt.expectedSender == memorySender && memorySent == 0 {
				t.Error("expected message to be sent via memorySender")
			}
			if tt.expectedSender == fallbackSender && fallbackSent == 0 {
				t.Error("expected message to be sent via fallbackSender")
			}
		})
	}
}

func TestSender_NilFallbackPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil fallback")
		}
	}()

	selector := func(topic string) message.Sender { return nil }
	multiplex.NewSender(selector, nil) // Should panic
}

func TestSender_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("send failed")

	failingSender := newMockSender()
	failingSender.sendErr = expectedErr

	fallbackSender := newMockSender()
	selector := multiplex.PrefixSenderSelector("fail", failingSender)
	mux := multiplex.NewSender(selector, fallbackSender)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	err := mux.Send(ctx, "fail/topic", []*message.Message{msg})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// ============================================================================
// Receiver Tests
// ============================================================================

func TestReceiver_Basic(t *testing.T) {
	memoryReceiver := newMockReceiver()
	fallbackReceiver := newMockReceiver()

	selector := multiplex.PrefixReceiverSelector("internal", memoryReceiver)
	mux := multiplex.NewReceiver(selector, fallbackReceiver)

	ctx := context.Background()

	tests := []struct {
		topic            string
		expectedReceiver *mockReceiver
	}{
		{"internal/events", memoryReceiver},
		{"internal/cache", memoryReceiver},
		{"external/api", fallbackReceiver},
		{"orders/created", fallbackReceiver},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			// Add message to expected receiver
			expectedMsg := message.New([]byte("test-msg"), message.Attributes{})
			tt.expectedReceiver.addMessages(tt.topic, expectedMsg)

			msgs, err := mux.Receive(ctx, tt.topic)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(msgs) == 0 {
				t.Error("expected to receive messages")
			}
		})
	}
}

func TestReceiver_NilFallbackPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil fallback")
		}
	}()

	selector := func(topic string) message.Receiver { return nil }
	multiplex.NewReceiver(selector, nil) // Should panic
}

// ============================================================================
// Exact Match Tests (no pattern matching)
// ============================================================================

func TestTopicExactMatch(t *testing.T) {
	tests := []struct {
		routeTopic string
		topic      string
		matches    bool
	}{
		// Exact match
		{"orders/created", "orders/created", true},
		{"orders/created", "orders/updated", false},

		// Different segments
		{"orders/us/created", "orders/us/created", true},
		{"orders/us/created", "orders/eu/created", false},

		// No wildcards
		{"orders", "orders", true},
		{"orders", "orders/created", false},
	}

	for _, tt := range tests {
		t.Run(tt.topic+"_vs_"+tt.routeTopic, func(t *testing.T) {
			sender := newMockSender()
			fallback := newMockSender()

			selector := multiplex.NewTopicSenderSelector([]multiplex.TopicSenderRoute{
				{Topic: tt.routeTopic, Sender: sender},
			})

			mux := multiplex.NewSender(selector, fallback)

			ctx := context.Background()
			msg := message.New([]byte("test"), message.Attributes{})

			err := mux.Send(ctx, tt.topic, []*message.Message{msg})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			senderGotMessage := len(sender.getSent(tt.topic)) > 0
			fallbackGotMessage := len(fallback.getSent(tt.topic)) > 0

			if tt.matches && !senderGotMessage {
				t.Errorf("expected route %q to match topic %q, but it didn't", tt.routeTopic, tt.topic)
			}
			if !tt.matches && !fallbackGotMessage {
				t.Errorf("expected route %q to NOT match topic %q (should use fallback), but it did", tt.routeTopic, tt.topic)
			}
		})
	}
}

func TestExactMatch_FirstMatchWins(t *testing.T) {
	sender1 := newMockSender()
	sender2 := newMockSender()
	fallback := newMockSender()

	selector := multiplex.NewTopicSenderSelector([]multiplex.TopicSenderRoute{
		{Topic: "orders/created", Sender: sender1}, // Should match first
		{Topic: "orders/created", Sender: sender2}, // Duplicate, should never be reached
	})

	mux := multiplex.NewSender(selector, fallback)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	err := mux.Send(ctx, "orders/created", []*message.Message{msg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should go to sender1 (first match)
	if len(sender1.getSent("orders/created")) == 0 {
		t.Error("expected sender1 to receive message (first match)")
	}
	if len(sender2.getSent("orders/created")) > 0 {
		t.Error("sender2 should not receive message (second route should not be checked)")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestPrefixSelector(t *testing.T) {
	sender := newMockSender()
	fallback := newMockSender()

	selector := multiplex.PrefixSenderSelector("internal", sender)
	mux := multiplex.NewSender(selector, fallback)

	tests := []struct {
		topic          string
		expectedSender *mockSender
	}{
		{"internal", sender},
		{"internal/cache", sender},
		{"internal/events/created", sender},
		{"external", fallback},
		{"intl", fallback},
	}

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			sender.sent = make(map[string][][]*message.Message)
			fallback.sent = make(map[string][][]*message.Message)

			err := mux.Send(ctx, tt.topic, []*message.Message{msg})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			senderGot := len(sender.getSent(tt.topic)) > 0
			fallbackGot := len(fallback.getSent(tt.topic)) > 0

			if tt.expectedSender == sender && !senderGot {
				t.Error("expected sender to receive message")
			}
			if tt.expectedSender == fallback && !fallbackGot {
				t.Error("expected fallback to receive message")
			}
		})
	}
}

func TestChainedSelectors(t *testing.T) {
	auditSender := newMockSender()
	internalSender := newMockSender()
	fallback := newMockSender()

	selector := multiplex.ChainSenderSelectors(
		multiplex.PrefixSenderSelector("audit", auditSender),
		multiplex.PrefixSenderSelector("internal", internalSender),
	)

	mux := multiplex.NewSender(selector, fallback)

	tests := []struct {
		topic          string
		expectedSender *mockSender
	}{
		{"audit/log", auditSender},
		{"audit/us/log", auditSender},
		{"internal/cache", internalSender},
		{"internal/events", internalSender},
		{"external/api", fallback},
	}

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			auditSender.sent = make(map[string][][]*message.Message)
			internalSender.sent = make(map[string][][]*message.Message)
			fallback.sent = make(map[string][][]*message.Message)

			err := mux.Send(ctx, tt.topic, []*message.Message{msg})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			auditGot := len(auditSender.getSent(tt.topic)) > 0
			internalGot := len(internalSender.getSent(tt.topic)) > 0
			fallbackGot := len(fallback.getSent(tt.topic)) > 0

			if tt.expectedSender == auditSender && !auditGot {
				t.Error("expected auditSender to receive message")
			}
			if tt.expectedSender == internalSender && !internalGot {
				t.Error("expected internalSender to receive message")
			}
			if tt.expectedSender == fallback && !fallbackGot {
				t.Error("expected fallback to receive message")
			}
		})
	}
}

func TestChainedSelectors_FirstMatchWins(t *testing.T) {
	sender1 := newMockSender()
	sender2 := newMockSender()
	fallback := newMockSender()

	// Both match "internal/*" but first one should win
	selector := multiplex.ChainSenderSelectors(
		multiplex.PrefixSenderSelector("internal", sender1),
		multiplex.PrefixSenderSelector("internal", sender2), // This should never be reached
	)

	mux := multiplex.NewSender(selector, fallback)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	err := mux.Send(ctx, "internal/cache", []*message.Message{msg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender1.getSent("internal/cache")) == 0 {
		t.Error("expected sender1 (first match) to receive message")
	}
	if len(sender2.getSent("internal/cache")) > 0 {
		t.Error("sender2 should not receive message (second selector should not be checked)")
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestIntegration_WithPublisher(t *testing.T) {
	// Create brokers
	memoryBroker := broker.NewChannelBroker(broker.ChannelBrokerConfig{})
	externalBroker := broker.NewChannelBroker(broker.ChannelBrokerConfig{})

	// Create multiplex sender
	selector := multiplex.PrefixSenderSelector("internal", memoryBroker)
	multiplexSender := multiplex.NewSender(selector, externalBroker)

	// Create publisher using multiplex sender
	publisher := message.NewPublisher(
		multiplexSender,
		message.PublisherConfig{
			MaxBatchSize: 10,
		},
	)

	ctx := context.Background()
	msgs := make(chan *message.Message, 10)

	// Start publisher
	done := publisher.Publish(ctx, msgs)

	// Send messages with different topics
	msgs <- message.New([]byte("internal-1"), message.Attributes{
		message.AttrTopic: "internal/cache",
	})
	msgs <- message.New([]byte("external-1"), message.Attributes{
		message.AttrTopic: "external/api",
	})

	close(msgs)
	<-done

	// Basic integration test completed successfully
}

func TestIntegration_WithSubscriber(t *testing.T) {
	// Create brokers
	memoryBroker := broker.NewChannelBroker(broker.ChannelBrokerConfig{})
	externalBroker := broker.NewChannelBroker(broker.ChannelBrokerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Subscribe first (messages only delivered to active subscriptions)
	internalCh := memoryBroker.Subscribe(ctx, "internal/events")
	externalCh := externalBroker.Subscribe(ctx, "external/api")

	// Send messages to brokers
	if err := memoryBroker.Send(ctx, "internal/events", []*message.Message{
		message.New([]byte("internal-msg"), message.Attributes{}),
	}); err != nil {
		t.Fatalf("Failed to send to memory broker: %v", err)
	}
	if err := externalBroker.Send(ctx, "external/api", []*message.Message{
		message.New([]byte("external-msg"), message.Attributes{}),
	}); err != nil {
		t.Fatalf("Failed to send to external broker: %v", err)
	}

	// Read from subscriptions with timeout
	select {
	case msg := <-internalCh:
		if msg == nil {
			t.Error("expected internal message, got nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for internal message")
	}

	select {
	case msg := <-externalCh:
		if msg == nil {
			t.Error("expected external message, got nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for external message")
	}
}
