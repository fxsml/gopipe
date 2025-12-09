package message_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// Mock sender for testing
type mockSender struct {
	mu       sync.Mutex
	sent     map[string][][]*message.Message
	sendErr  error
	sendFunc func(ctx context.Context, topic string, msgs []*message.Message) error
}

func newMockSender() *mockSender {
	return &mockSender{
		sent: make(map[string][][]*message.Message),
	}
}

func (m *mockSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, topic, msgs)
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent[topic] = append(m.sent[topic], msgs)
	return nil
}

func (m *mockSender) getSent(topic string) [][]*message.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[topic]
}

// Mock receiver for testing
type mockReceiver struct {
	mu          sync.Mutex
	messages    map[string][]*message.Message
	receiveErr  error
	receiveFunc func(ctx context.Context, topic string) ([]*message.Message, error)
}

func newMockReceiver() *mockReceiver {
	return &mockReceiver{
		messages: make(map[string][]*message.Message),
	}
}

func (m *mockReceiver) addMessages(topic string, msgs ...*message.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[topic] = append(m.messages[topic], msgs...)
}

func (m *mockReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	if m.receiveFunc != nil {
		return m.receiveFunc(ctx, topic)
	}
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages[topic]) == 0 {
		return nil, nil
	}
	msgs := m.messages[topic]
	m.messages[topic] = nil
	return msgs, nil
}

func TestPublisher_Basic(t *testing.T) {
	sender := newMockSender()
	publisher := message.NewPublisher(
		sender,
		message.RouteBySubject(),
		message.PublisherConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		msgs <- message.New([]byte("msg1"), message.Properties{message.PropSubject: "topic-a"})
		msgs <- message.New([]byte("msg2"), message.Properties{message.PropSubject: "topic-a"})
		msgs <- message.New([]byte("msg3"), message.Properties{message.PropSubject: "topic-b"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	<-publisher.Publish(ctx, msgs)

	// Verify messages were sent
	topicA := sender.getSent("topic-a")
	topicB := sender.getSent("topic-b")

	if len(topicA) == 0 {
		t.Error("Expected messages for topic-a")
	}
	if len(topicB) == 0 {
		t.Error("Expected messages for topic-b")
	}

	// Check message count
	var countA, countB int
	for _, batch := range topicA {
		countA += len(batch)
	}
	for _, batch := range topicB {
		countB += len(batch)
	}

	if countA != 2 {
		t.Errorf("Expected 2 messages for topic-a, got %d", countA)
	}
	if countB != 1 {
		t.Errorf("Expected 1 message for topic-b, got %d", countB)
	}
}

func TestPublisher_Batching(t *testing.T) {
	sender := newMockSender()
	publisher := message.NewPublisher(
		sender,
		message.RouteStatic("topic"),
		message.PublisherConfig{
			MaxBatchSize: 3,
			MaxDuration:  time.Hour,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		for i := 0; i < 10; i++ {
			msgs <- message.New([]byte("msg"), nil)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	<-publisher.Publish(ctx, msgs)

	batches := sender.getSent("topic")
	// Should have 3 full batches of 3 and 1 partial batch of 1
	if len(batches) != 4 {
		t.Errorf("Expected 4 batches, got %d", len(batches))
	}
}

func TestPublisher_ErrorHandling(t *testing.T) {
	testErr := errors.New("send error")
	sender := newMockSender()
	sender.sendErr = testErr

	publisher := message.NewPublisher(
		sender,
		message.RouteStatic("topic"),
		message.PublisherConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
			Retry: &gopipe.RetryConfig{
				MaxAttempts: 2,
				Backoff:     gopipe.ConstantBackoff(time.Millisecond, 0),
			},
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		msgs <- message.New([]byte("msg"), nil)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	<-publisher.Publish(ctx, msgs)
	// Should complete even with errors due to retry exhaustion
}

func TestPublisher_Concurrency(t *testing.T) {
	var sendCount int
	var mu sync.Mutex

	sender := newMockSender()
	sender.sendFunc = func(ctx context.Context, topic string, msgs []*message.Message) error {
		mu.Lock()
		sendCount++
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	publisher := message.NewPublisher(
		sender,
		message.RouteBySubject(),
		message.PublisherConfig{
			MaxBatchSize: 1,
			MaxDuration:  time.Hour,
			Concurrency:  3,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		for i := 0; i < 10; i++ {
			msgs <- message.New([]byte("msg"), message.Properties{message.PropSubject: "topic-a"})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	<-publisher.Publish(ctx, msgs)

	if sendCount != 10 {
		t.Errorf("Expected 10 sends, got %d", sendCount)
	}
}

func TestSubscriber_Basic(t *testing.T) {
	receiver := newMockReceiver()
	receiver.addMessages("topic-a",
		message.New([]byte("msg1"), nil),
		message.New([]byte("msg2"), nil),
	)

	var callCount int
	receiver.receiveFunc = func(ctx context.Context, topic string) ([]*message.Message, error) {
		callCount++
		if callCount == 1 {
			return receiver.messages[topic], nil
		}
		// Block on subsequent calls to simulate waiting for messages
		<-ctx.Done()
		return nil, ctx.Err()
	}

	subscriber := message.NewSubscriber(
		receiver,
		message.SubscriberConfig{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs := subscriber.Subscribe(ctx, "topic-a")

	var count int
	for range msgs {
		count++
		if count == 2 {
			cancel()
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 messages, got %d", count)
	}
}

func TestSubscriber_ErrorHandling(t *testing.T) {
	testErr := errors.New("receive error")
	receiver := newMockReceiver()
	receiver.receiveErr = testErr

	subscriber := message.NewSubscriber(
		receiver,
		message.SubscriberConfig{
			Retry: &gopipe.RetryConfig{
				MaxAttempts: 3,
				Backoff:     gopipe.ConstantBackoff(time.Millisecond, 0),
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs := subscriber.Subscribe(ctx, "topic-a")

	// Should receive no messages due to errors
	for range msgs {
		t.Error("Expected no messages due to errors")
	}
}

func TestSubscriber_MultipleReceives(t *testing.T) {
	receiver := newMockReceiver()
	callCount := 0

	receiver.receiveFunc = func(ctx context.Context, topic string) ([]*message.Message, error) {
		callCount++
		switch callCount {
		case 1:
			return []*message.Message{message.New([]byte("msg1"), nil)}, nil
		case 2:
			return []*message.Message{message.New([]byte("msg2"), nil)}, nil
		default:
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}

	subscriber := message.NewSubscriber(
		receiver,
		message.SubscriberConfig{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	msgs := subscriber.Subscribe(ctx, "topic-a")

	var count int
	for range msgs {
		count++
		if count == 2 {
			cancel()
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 messages, got %d", count)
	}
}

func TestPublisher_WithRecover(t *testing.T) {
	sender := newMockSender()
	sender.sendFunc = func(ctx context.Context, topic string, msgs []*message.Message) error {
		panic("send panic")
	}

	publisher := message.NewPublisher(
		sender,
		message.RouteStatic("topic"),
		message.PublisherConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
			Recover:      true,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		msgs <- message.New([]byte("msg"), nil)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Should not panic due to Recover option
	<-publisher.Publish(ctx, msgs)
}

func TestSubscriber_WithRecover(t *testing.T) {
	receiver := newMockReceiver()
	receiver.receiveFunc = func(ctx context.Context, topic string) ([]*message.Message, error) {
		panic("receive panic")
	}

	subscriber := message.NewSubscriber(
		receiver,
		message.SubscriberConfig{
			Recover: true,
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs := subscriber.Subscribe(ctx, "topic-a")

	// Should not panic due to Recover option
	for range msgs {
		t.Error("Expected no messages due to panic")
	}
}

// ============================================================================
// Routing Key Helper Tests
// ============================================================================

func TestRouteBySubject(t *testing.T) {
	route := message.RouteBySubject()

	tests := []struct {
		name       string
		properties message.Properties
		expected   string
	}{
		{
			name:       "with subject",
			properties: message.Properties{message.PropSubject: "orders"},
			expected:   "orders",
		},
		{
			name:       "without subject",
			properties: message.Properties{},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := route(tt.properties)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRouteByProperty(t *testing.T) {
	route := message.RouteByProperty("tenant-id")

	tests := []struct {
		name       string
		properties message.Properties
		expected   string
	}{
		{
			name:       "with property",
			properties: message.Properties{"tenant-id": "tenant-123"},
			expected:   "tenant-123",
		},
		{
			name:       "without property",
			properties: message.Properties{},
			expected:   "",
		},
		{
			name:       "with non-string property",
			properties: message.Properties{"tenant-id": 123},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := route(tt.properties)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRouteStatic(t *testing.T) {
	route := message.RouteStatic("fixed-topic")

	tests := []struct {
		name       string
		properties message.Properties
		expected   string
	}{
		{
			name:       "empty properties",
			properties: message.Properties{},
			expected:   "fixed-topic",
		},
		{
			name:       "with properties",
			properties: message.Properties{"key": "value"},
			expected:   "fixed-topic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := route(tt.properties)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRouteByFormat(t *testing.T) {
	route := message.RouteByFormat("tenant-%s-events", "tenant-id")

	tests := []struct {
		name       string
		properties message.Properties
		expected   string
	}{
		{
			name:       "with property",
			properties: message.Properties{"tenant-id": "123"},
			expected:   "tenant-123-events",
		},
		{
			name:       "without property",
			properties: message.Properties{},
			expected:   "tenant-%!s(<nil>)-events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := route(tt.properties)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRouteByFormat_MultipleProperties(t *testing.T) {
	route := message.RouteByFormat("env-%s-tenant-%s-events", "environment", "tenant-id")

	result := route(message.Properties{
		"environment": "prod",
		"tenant-id":   "abc",
	})

	expected := "env-prod-tenant-abc-events"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
