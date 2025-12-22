package message_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe/middleware"
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
		message.PublisherConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		msgs <- message.New([]byte("msg1"), message.Attributes{message.AttrTopic: "topic-a"})
		msgs <- message.New([]byte("msg2"), message.Attributes{message.AttrTopic: "topic-a"})
		msgs <- message.New([]byte("msg3"), message.Attributes{message.AttrTopic: "topic-b"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done, _ := publisher.Publish(ctx, msgs)
	<-done

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
		message.PublisherConfig{
			MaxBatchSize: 3,
			MaxDuration:  time.Hour,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		for i := 0; i < 10; i++ {
			msgs <- message.New([]byte("msg"), message.Attributes{message.AttrTopic: "topic"})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done, _ := publisher.Publish(ctx, msgs)
	<-done

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
		message.PublisherConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
			Retry: &middleware.RetryConfig{
				MaxAttempts: 2,
				Backoff:     middleware.ConstantBackoff(time.Millisecond, 0),
			},
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		msgs <- message.New([]byte("msg"), message.Attributes{message.AttrTopic: "topic"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done, _ := publisher.Publish(ctx, msgs)
	<-done
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
			msgs <- message.New([]byte("msg"), message.Attributes{message.AttrTopic: "topic-a"})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done, _ := publisher.Publish(ctx, msgs)
	<-done

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

	msgs, _ := subscriber.Subscribe(ctx, "topic-a")

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
			Retry: &middleware.RetryConfig{
				MaxAttempts: 3,
				Backoff:     middleware.ConstantBackoff(time.Millisecond, 0),
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs, _ := subscriber.Subscribe(ctx, "topic-a")

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

	msgs, _ := subscriber.Subscribe(ctx, "topic-a")

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
		message.PublisherConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
			Recover:      true,
		},
	)

	msgs := make(chan *message.Message)
	go func() {
		defer close(msgs)
		msgs <- message.New([]byte("msg"), message.Attributes{message.AttrTopic: "topic"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Should not panic due to Recover option
	done, _ := publisher.Publish(ctx, msgs)
	<-done
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

	msgs, _ := subscriber.Subscribe(ctx, "topic-a")

	// Should not panic due to Recover option
	for range msgs {
		t.Error("Expected no messages due to panic")
	}
}

func TestSubscriber_MultipleTopics(t *testing.T) {
	receiver := newMockReceiver()
	receiver.addMessages("topic-a", message.New([]byte("msg-a"), nil))
	receiver.addMessages("topic-b", message.New([]byte("msg-b"), nil))

	var mu sync.Mutex
	callCount := make(map[string]int)

	receiver.receiveFunc = func(ctx context.Context, topic string) ([]*message.Message, error) {
		mu.Lock()
		callCount[topic]++
		count := callCount[topic]
		mu.Unlock()

		if count == 1 {
			receiver.mu.Lock()
			msgs := receiver.messages[topic]
			receiver.messages[topic] = nil
			receiver.mu.Unlock()
			return msgs, nil
		}
		// Block on subsequent calls
		<-ctx.Done()
		return nil, ctx.Err()
	}

	subscriber := message.NewSubscriber(
		receiver,
		message.SubscriberConfig{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Subscribe to multiple topics and merge them
	topicA, _ := subscriber.Subscribe(ctx, "topic-a")
	topicB, _ := subscriber.Subscribe(ctx, "topic-b")
	msgs := channel.Merge(topicA, topicB)

	var received []string
	for msg := range msgs {
		received = append(received, string(msg.Data))
		if len(received) == 2 {
			cancel()
		}
	}

	if len(received) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(received))
	}

	// Verify we got messages from both topics
	hasA := false
	hasB := false
	for _, payload := range received {
		if payload == "msg-a" {
			hasA = true
		}
		if payload == "msg-b" {
			hasB = true
		}
	}
	if !hasA {
		t.Error("Expected message from topic-a")
	}
	if !hasB {
		t.Error("Expected message from topic-b")
	}
}

func TestSubscriber_NoMessages(t *testing.T) {
	receiver := newMockReceiver()
	receiver.receiveFunc = func(ctx context.Context, topic string) ([]*message.Message, error) {
		// Block forever - no messages to return
		<-ctx.Done()
		return nil, ctx.Err()
	}

	subscriber := message.NewSubscriber(
		receiver,
		message.SubscriberConfig{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs, _ := subscriber.Subscribe(ctx, "topic-a")

	var count int
	for range msgs {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 messages when receiver returns no messages, got %d", count)
	}
}

