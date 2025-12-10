package pubsub_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
)

// ============================================================================
// Sender Interface Tests - Apply to any Sender implementation
// ============================================================================

// SenderTestSuite runs standard tests against any Sender implementation.
type SenderTestSuite struct {
	NewSender func() pubsub.Sender
}

func (s *SenderTestSuite) Run(t *testing.T) {
	t.Run("Send_SingleMessage", s.TestSendSingleMessage)
	t.Run("Send_MultipleMessages", s.TestSendMultipleMessages)
	t.Run("Send_EmptySlice", s.TestSendEmptySlice)
	t.Run("Send_ContextCanceled", s.TestSendContextCanceled)
	t.Run("Send_DifferentTopics", s.TestSendDifferentTopics)
}

func (s *SenderTestSuite) TestSendSingleMessage(t *testing.T) {
	sender := s.NewSender()
	ctx := context.Background()
	msg := message.New([]byte("test payload"), message.Attributes{"key": "value"})

	err := sender.Send(ctx, "test/topic", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func (s *SenderTestSuite) TestSendMultipleMessages(t *testing.T) {
	sender := s.NewSender()
	ctx := context.Background()
	msgs := []*message.Message{
		message.New([]byte("msg1"), message.Attributes{}),
		message.New([]byte("msg2"), message.Attributes{}),
		message.New([]byte("msg3"), message.Attributes{}),
	}

	err := sender.Send(ctx, "test/topic", msgs)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func (s *SenderTestSuite) TestSendEmptySlice(t *testing.T) {
	sender := s.NewSender()
	ctx := context.Background()

	err := sender.Send(ctx, "test/topic", []*message.Message{})
	if err != nil {
		t.Fatalf("Send empty slice should not fail: %v", err)
	}
}

func (s *SenderTestSuite) TestSendContextCanceled(t *testing.T) {
	sender := s.NewSender()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msg := message.New([]byte("test"), message.Attributes{})
	err := sender.Send(ctx, "test/topic", []*message.Message{msg})

	// Should return context error (or be a no-op for some implementations)
	// We don't require specific error, just that it doesn't hang
	_ = err
}

func (s *SenderTestSuite) TestSendDifferentTopics(t *testing.T) {
	sender := s.NewSender()
	ctx := context.Background()

	topics := []string{"topic/a", "topic/b", "topic/c/nested"}
	for _, topic := range topics {
		msg := message.New([]byte(topic), message.Attributes{})
		err := sender.Send(ctx, topic, []*message.Message{msg})
		if err != nil {
			t.Fatalf("Send to %s failed: %v", topic, err)
		}
	}
}

// ============================================================================
// Receiver Interface Tests - Apply to any Receiver implementation
// ============================================================================

// ReceiverTestSuite runs standard tests against any Receiver implementation.
type ReceiverTestSuite struct {
	NewReceiver func() pubsub.Receiver
}

func (s *ReceiverTestSuite) Run(t *testing.T) {
	t.Run("Receive_EmptyTopic", s.TestReceiveEmptyTopic)
	t.Run("Receive_ContextCanceled", s.TestReceiveContextCanceled)
	t.Run("Receive_DifferentTopics", s.TestReceiveDifferentTopics)
}

func (s *ReceiverTestSuite) TestReceiveEmptyTopic(t *testing.T) {
	receiver := s.NewReceiver()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	msgs, err := receiver.Receive(ctx, "nonexistent/topic")
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Receive failed: %v", err)
	}
	// Empty result is expected for nonexistent topic
	_ = msgs
}

func (s *ReceiverTestSuite) TestReceiveContextCanceled(t *testing.T) {
	receiver := s.NewReceiver()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := receiver.Receive(ctx, "test/topic")
	// Should return quickly (context canceled)
	_ = err
}

func (s *ReceiverTestSuite) TestReceiveDifferentTopics(t *testing.T) {
	receiver := s.NewReceiver()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	topics := []string{"topic/a", "topic/b", "topic/c"}
	for _, topic := range topics {
		msgs, err := receiver.Receive(ctx, topic)
		if err != nil && err != context.DeadlineExceeded {
			t.Fatalf("Receive from %s failed: %v", topic, err)
		}
		_ = msgs
	}
}

// ============================================================================
// ChannelBroker Tests (combined Sender + Receiver with Subscribe)
// ============================================================================

func TestChannelBroker_Subscribe_SendReceive(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Subscribe first
	ch := broker.Subscribe(ctx, "test/topic")

	// Send message
	msg := message.New([]byte("hello"), message.Attributes{"id": "1"})
	err := broker.Send(ctx, "test/topic", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive from subscription channel
	select {
	case received := <-ch:
		if !bytes.Equal(received.Data, []byte("hello")) {
			t.Errorf("Expected 'hello', got %s", received.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for message")
	}
}

func TestChannelBroker_Subscribe_MultipleSubscribers(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create multiple subscribers to same topic
	ch1 := broker.Subscribe(ctx, "events")
	ch2 := broker.Subscribe(ctx, "events")
	ch3 := broker.Subscribe(ctx, "events")

	// Send one message
	msg := message.New([]byte("event"), message.Attributes{})
	err := broker.Send(ctx, "events", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// All subscribers should receive the message
	for i, ch := range []<-chan *message.Message{ch1, ch2, ch3} {
		select {
		case received := <-ch:
			if !bytes.Equal(received.Data, []byte("event")) {
				t.Errorf("Subscriber %d: expected 'event', got %s", i+1, received.Data)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Subscriber %d: timeout waiting for message", i+1)
		}
	}
}

func TestChannelBroker_Subscribe_DifferentTopics(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Subscribe to different topics
	chOrders := broker.Subscribe(ctx, "orders")
	chUsers := broker.Subscribe(ctx, "users")

	// Send to orders topic
	broker.Send(ctx, "orders", []*message.Message{
		message.New([]byte("order-msg"), message.Attributes{}),
	})

	// Send to users topic
	broker.Send(ctx, "users", []*message.Message{
		message.New([]byte("user-msg"), message.Attributes{}),
	})

	// Check orders channel
	select {
	case msg := <-chOrders:
		if !bytes.Equal(msg.Data, []byte("order-msg")) {
			t.Errorf("Orders: expected 'order-msg', got %s", msg.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for orders message")
	}

	// Check users channel
	select {
	case msg := <-chUsers:
		if !bytes.Equal(msg.Data, []byte("user-msg")) {
			t.Errorf("Users: expected 'user-msg', got %s", msg.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for users message")
	}
}

func TestChannelBroker_Subscribe_ExactMatchOnly(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Subscribe to specific topic
	ch := broker.Subscribe(ctx, "orders/created")

	// Send to different topics
	broker.Send(ctx, "orders", []*message.Message{
		message.New([]byte("parent"), message.Attributes{}),
	})
	broker.Send(ctx, "orders/created/v2", []*message.Message{
		message.New([]byte("child"), message.Attributes{}),
	})
	broker.Send(ctx, "orders/created", []*message.Message{
		message.New([]byte("exact"), message.Attributes{}),
	})

	// Should only receive exact match
	select {
	case msg := <-ch:
		if !bytes.Equal(msg.Data, []byte("exact")) {
			t.Errorf("Expected 'exact', got %s", msg.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for message")
	}

	// Should not receive more messages
	select {
	case msg := <-ch:
		t.Errorf("Unexpected message: %s", msg.Data)
	case <-time.After(50 * time.Millisecond):
		// Expected - no more messages
	}
}

func TestChannelBroker_Subscribe_ContextCancel(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch := broker.Subscribe(ctx, "test")

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			// Message received before close, that's ok
		}
		// Channel closed, as expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Channel should close when context is canceled")
	}
}

func TestChannelBroker_Close(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})

	ctx := context.Background()
	ch := broker.Subscribe(ctx, "test")

	// Close broker
	err := broker.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Channel should close when broker is closed")
	}

	// Send after close should fail
	msg := message.New([]byte("test"), message.Attributes{})
	err = broker.Send(ctx, "test", []*message.Message{msg})
	if err != pubsub.ErrBrokerClosed {
		t.Errorf("Expected ErrBrokerClosed, got %v", err)
	}

	// Double close should return error
	err = broker.Close()
	if err != pubsub.ErrBrokerClosed {
		t.Errorf("Expected ErrBrokerClosed on double close, got %v", err)
	}
}

func TestChannelBroker_Receive_Polling(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start a goroutine to send after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		broker.Send(ctx, "test", []*message.Message{
			message.New([]byte("delayed"), message.Attributes{}),
		})
	}()

	// Receive should pick up the message
	msgs, err := broker.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}
	if len(msgs) > 0 && !bytes.Equal(msgs[0].Data, []byte("delayed")) {
		t.Errorf("Expected 'delayed', got %s", msgs[0].Data)
	}
}

func TestChannelBroker_ConcurrentSendSubscribe(t *testing.T) {
	broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 100})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numSenders = 10
	const numMessages = 100
	const topic = "concurrent"

	// Subscribe
	ch := broker.Subscribe(ctx, topic)

	// Start senders
	var wg sync.WaitGroup
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				msg := message.New([]byte("msg"), message.Attributes{})
				broker.Send(ctx, topic, []*message.Message{msg})
			}
		}(i)
	}

	// Collect messages
	received := 0
	expected := numSenders * numMessages

	go func() {
		wg.Wait()
		// Give some time for all messages to be delivered
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				goto done
			}
			received++
		case <-ctx.Done():
			goto done
		}
	}
done:

	if received != expected {
		t.Errorf("Expected %d messages, received %d", expected, received)
	}
}

// ============================================================================
// Run Sender/Receiver test suites against ChannelBroker
// ============================================================================

func TestChannelBroker_SenderInterface(t *testing.T) {
	suite := &SenderTestSuite{
		NewSender: func() pubsub.Sender {
			return pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
		},
	}
	suite.Run(t)
}

func TestChannelBroker_ReceiverInterface(t *testing.T) {
	suite := &ReceiverTestSuite{
		NewReceiver: func() pubsub.Receiver {
			return pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{BufferSize: 10})
		},
	}
	suite.Run(t)
}

// ============================================================================
// HTTPReceiver Tests
// ============================================================================

func TestHTTPReceiver_MessageClearing(t *testing.T) {
	receiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx := context.Background()

	// Simulate receiving messages by calling Receive on empty receiver
	msgs, err := receiver.Receive(ctx, "test/topic")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(msgs))
	}
}

func TestHTTPReceiver_ReceiverInterface(t *testing.T) {
	suite := &ReceiverTestSuite{
		NewReceiver: func() pubsub.Receiver {
			return pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)
		},
	}
	suite.Run(t)
}

// ============================================================================
// IOBroker Tests
// ============================================================================

func TestIOBroker_SenderInterface(t *testing.T) {
	// IOBroker requires valid JSON payloads, so we test it separately
	var buf bytes.Buffer
	sender := pubsub.NewIOSender(&buf, pubsub.IOConfig{})
	ctx := context.Background()

	// Test with valid JSON payload
	msg := message.New([]byte(`{"test": "payload"}`), message.Attributes{"key": "value"})
	err := sender.Send(ctx, "test/topic", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Test multiple messages
	msgs := []*message.Message{
		message.New([]byte(`"msg1"`), message.Attributes{}),
		message.New([]byte(`"msg2"`), message.Attributes{}),
	}
	err = sender.Send(ctx, "test/topic", msgs)
	if err != nil {
		t.Fatalf("Send multiple failed: %v", err)
	}

	// Test empty slice
	err = sender.Send(ctx, "test/topic", []*message.Message{})
	if err != nil {
		t.Fatalf("Send empty failed: %v", err)
	}
}

func TestIOBroker_ReceiverInterface(t *testing.T) {
	suite := &ReceiverTestSuite{
		NewReceiver: func() pubsub.Receiver {
			// Empty reader
			return pubsub.NewIOReceiver(bytes.NewReader(nil), pubsub.IOConfig{})
		},
	}
	suite.Run(t)
}

// ============================================================================
// Topic Utility Tests
// ============================================================================

func TestSplitTopic(t *testing.T) {
	tests := []struct {
		topic    string
		expected []string
	}{
		{"", nil},
		{"orders", []string{"orders"}},
		{"orders/created", []string{"orders", "created"}},
		{"orders/us/created", []string{"orders", "us", "created"}},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			result := pubsub.SplitTopic(tt.topic)
			if len(result) != len(tt.expected) {
				t.Errorf("SplitTopic(%q) = %v, want %v", tt.topic, result, tt.expected)
			}
			for i := range result {
				if i < len(tt.expected) && result[i] != tt.expected[i] {
					t.Errorf("SplitTopic(%q)[%d] = %q, want %q", tt.topic, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestJoinTopic(t *testing.T) {
	tests := []struct {
		segments []string
		expected string
	}{
		{nil, ""},
		{[]string{"orders"}, "orders"},
		{[]string{"orders", "created"}, "orders/created"},
		{[]string{"orders", "us", "created"}, "orders/us/created"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := pubsub.JoinTopic(tt.segments...)
			if result != tt.expected {
				t.Errorf("JoinTopic(%v) = %q, want %q", tt.segments, result, tt.expected)
			}
		})
	}
}

func TestParentTopic(t *testing.T) {
	tests := []struct {
		topic    string
		expected string
	}{
		{"", ""},
		{"orders", ""},
		{"orders/created", "orders"},
		{"orders/us/created", "orders/us"},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			result := pubsub.ParentTopic(tt.topic)
			if result != tt.expected {
				t.Errorf("ParentTopic(%q) = %q, want %q", tt.topic, result, tt.expected)
			}
		})
	}
}

func TestBaseTopic(t *testing.T) {
	tests := []struct {
		topic    string
		expected string
	}{
		{"", ""},
		{"orders", "orders"},
		{"orders/created", "created"},
		{"orders/us/created", "created"},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			result := pubsub.BaseTopic(tt.topic)
			if result != tt.expected {
				t.Errorf("BaseTopic(%q) = %q, want %q", tt.topic, result, tt.expected)
			}
		})
	}
}
