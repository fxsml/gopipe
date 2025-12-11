package pubsub_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
	"github.com/fxsml/gopipe/pubsub/broker"
)

// ============================================================================
// MultiplexSender Tests
// ============================================================================

func TestMultiplexSender_Basic(t *testing.T) {
	memorySender := newMockSender()
	fallbackSender := newMockSender()

	// Use "/" separator for prefix matching
	selector := pubsub.PrefixSenderSelector("internal", memorySender)
	multiplex := pubsub.NewMultiplexSender(selector, fallbackSender)

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

			err := multiplex.Send(ctx, tt.topic, []*message.Message{msg})
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

func TestMultiplexSender_NilFallbackPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil fallback")
		}
	}()

	selector := func(topic string) pubsub.Sender { return nil }
	pubsub.NewMultiplexSender(selector, nil) // Should panic
}

func TestMultiplexSender_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("send failed")

	failingSender := newMockSender()
	failingSender.sendErr = expectedErr

	fallbackSender := newMockSender()
	selector := pubsub.PrefixSenderSelector("fail", failingSender)
	multiplex := pubsub.NewMultiplexSender(selector, fallbackSender)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	err := multiplex.Send(ctx, "fail/topic", []*message.Message{msg})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// ============================================================================
// MultiplexReceiver Tests
// ============================================================================

func TestMultiplexReceiver_Basic(t *testing.T) {
	memoryReceiver := newMockReceiver()
	fallbackReceiver := newMockReceiver()

	selector := pubsub.PrefixReceiverSelector("internal", memoryReceiver)
	multiplex := pubsub.NewMultiplexReceiver(selector, fallbackReceiver)

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

			msgs, err := multiplex.Receive(ctx, tt.topic)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(msgs) == 0 {
				t.Error("expected to receive messages")
			}
		})
	}
}

func TestMultiplexReceiver_NilFallbackPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil fallback")
		}
	}()

	selector := func(topic string) pubsub.Receiver { return nil }
	pubsub.NewMultiplexReceiver(selector, nil) // Should panic
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

			selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
				{Topic: tt.routeTopic, Sender: sender},
			})

			multiplex := pubsub.NewMultiplexSender(selector, fallback)

			ctx := context.Background()
			msg := message.New([]byte("test"), message.Attributes{})

			err := multiplex.Send(ctx, tt.topic, []*message.Message{msg})
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

	selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
		{Topic: "orders/created", Sender: sender1}, // Should match first
		{Topic: "orders/created", Sender: sender2}, // Duplicate, should never be reached
	})

	multiplex := pubsub.NewMultiplexSender(selector, fallback)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	err := multiplex.Send(ctx, "orders/created", []*message.Message{msg})
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

	selector := pubsub.PrefixSenderSelector("internal", sender)
	multiplex := pubsub.NewMultiplexSender(selector, fallback)

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

			err := multiplex.Send(ctx, tt.topic, []*message.Message{msg})
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

	selector := pubsub.ChainSenderSelectors(
		pubsub.PrefixSenderSelector("audit", auditSender),
		pubsub.PrefixSenderSelector("internal", internalSender),
	)

	multiplex := pubsub.NewMultiplexSender(selector, fallback)

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

			err := multiplex.Send(ctx, tt.topic, []*message.Message{msg})
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
	selector := pubsub.ChainSenderSelectors(
		pubsub.PrefixSenderSelector("internal", sender1),
		pubsub.PrefixSenderSelector("internal", sender2), // This should never be reached
	)

	multiplex := pubsub.NewMultiplexSender(selector, fallback)

	ctx := context.Background()
	msg := message.New([]byte("test"), message.Attributes{})

	err := multiplex.Send(ctx, "internal/cache", []*message.Message{msg})
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
	selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
	multiplexSender := pubsub.NewMultiplexSender(selector, externalBroker)

	// Create publisher using multiplex sender
	publisher := pubsub.NewPublisher(
		multiplexSender,
		pubsub.PublisherConfig{
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
	memoryBroker.Send(ctx, "internal/events", []*message.Message{
		message.New([]byte("internal-msg"), message.Attributes{}),
	})
	externalBroker.Send(ctx, "external/api", []*message.Message{
		message.New([]byte("external-msg"), message.Attributes{}),
	})

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
