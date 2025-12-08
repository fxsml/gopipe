package broker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func TestBroker_SendReceive(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start receiver before sending
	msgs := b.Receive(ctx, "test/topic")

	// Send message
	msg := message.New("hello", message.WithID[string]("msg-1"))
	if err := b.Send(ctx, "test/topic", msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive message
	select {
	case received := <-msgs:
		if received.Payload() != "hello" {
			t.Errorf("Expected payload 'hello', got '%s'", received.Payload())
		}
		if id, ok := received.ID(); !ok || id != "msg-1" {
			t.Errorf("Expected ID 'msg-1', got '%s'", id)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := broker.NewBroker[int](broker.Config{BufferSize: 10})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start multiple receivers
	recv1 := b.Receive(ctx, "events")
	recv2 := b.Receive(ctx, "events")
	recv3 := b.Receive(ctx, "events")

	// Give receivers time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Send message
	msg := message.New(42)
	if err := b.Send(ctx, "events", msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// All receivers should get the message
	var wg sync.WaitGroup
	wg.Add(3)

	checkReceiver := func(name string, ch <-chan *message.Message[int]) {
		defer wg.Done()
		select {
		case received := <-ch:
			if received.Payload() != 42 {
				t.Errorf("%s: Expected payload 42, got %d", name, received.Payload())
			}
		case <-ctx.Done():
			t.Errorf("%s: Timeout waiting for message", name)
		}
	}

	go checkReceiver("recv1", recv1)
	go checkReceiver("recv2", recv2)
	go checkReceiver("recv3", recv3)

	wg.Wait()
}

func TestBroker_MultipleTopics(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Subscribe to different topics
	ordersRecv := b.Receive(ctx, "orders/created")
	usersRecv := b.Receive(ctx, "users/updated")

	// Give receivers time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Send to different topics
	b.Send(ctx, "orders/created", message.New("order-123"))
	b.Send(ctx, "users/updated", message.New("user-456"))

	// Verify each receiver gets correct message
	select {
	case msg := <-ordersRecv:
		if msg.Payload() != "order-123" {
			t.Errorf("Orders: expected 'order-123', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for orders message")
	}

	select {
	case msg := <-usersRecv:
		if msg.Payload() != "user-456" {
			t.Errorf("Users: expected 'user-456', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for users message")
	}
}

func TestBroker_TopicsCreatedOnFly(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send to non-existent topic (should create it)
	err := b.Send(ctx, "new/topic/path", message.New("first"))
	if err != nil {
		t.Fatalf("Send to new topic failed: %v", err)
	}

	// Now subscribe and send again
	recv := b.Receive(ctx, "new/topic/path")
	time.Sleep(10 * time.Millisecond)

	err = b.Send(ctx, "new/topic/path", message.New("second"))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case msg := <-recv:
		if msg.Payload() != "second" {
			t.Errorf("Expected 'second', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

func TestBroker_Close(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{CloseTimeout: 100 * time.Millisecond})

	ctx := context.Background()
	recv := b.Receive(ctx, "test")

	// Close broker
	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Receiver channel should be closed
	select {
	case _, ok := <-recv:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for channel close")
	}

	// Operations should fail after close
	if err := b.Send(ctx, "test", message.New("msg")); err != broker.ErrBrokerClosed {
		t.Errorf("Expected ErrBrokerClosed, got %v", err)
	}

	// Double close should return error
	if err := b.Close(); err != broker.ErrBrokerClosed {
		t.Errorf("Expected ErrBrokerClosed on double close, got %v", err)
	}
}

func TestBroker_ContextCancellation(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	recv := b.Receive(ctx, "test")

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-recv:
		if ok {
			t.Error("Expected channel to be closed after context cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for channel close")
	}
}

func TestBroker_SendTimeout(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{
		SendTimeout: 10 * time.Millisecond,
	})
	defer b.Close()

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Send(ctx, "test", message.New("msg"))
	if err == nil {
		t.Error("Expected error on send with canceled context")
	}
}

func TestNewSender(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	sender := broker.NewSender(b)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Should be able to send via sender interface
	err := sender.Send(ctx, "test", message.New("hello"))
	if err != nil {
		t.Fatalf("Send via Sender interface failed: %v", err)
	}
}

func TestNewReceiver(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	receiver := broker.NewReceiver(b)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	recv := receiver.Receive(ctx, "test")
	time.Sleep(10 * time.Millisecond)

	// Send via broker
	b.Send(ctx, "test", message.New("hello"))

	// Should receive via receiver interface
	select {
	case msg := <-recv:
		if msg.Payload() != "hello" {
			t.Errorf("Expected 'hello', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}
}

func TestBroker_BatchSend(t *testing.T) {
	b := broker.NewBroker[int](broker.Config{BufferSize: 100})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	recv := b.Receive(ctx, "batch")
	time.Sleep(10 * time.Millisecond)

	// Send multiple messages at once
	msgs := []*message.Message[int]{
		message.New(1),
		message.New(2),
		message.New(3),
	}
	if err := b.Send(ctx, "batch", msgs...); err != nil {
		t.Fatalf("Batch send failed: %v", err)
	}

	// Receive all messages
	received := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case msg := <-recv:
			received = append(received, msg.Payload())
		case <-ctx.Done():
			t.Fatalf("Timeout after receiving %d messages", len(received))
		}
	}

	if len(received) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(received))
	}
}

func TestBroker_HierarchicalTopics(t *testing.T) {
	b := broker.NewBroker[string](broker.Config{})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Subscribe to specific hierarchical topics
	ordersCreated := b.Receive(ctx, "orders/created")
	ordersUpdated := b.Receive(ctx, "orders/updated")
	usersProfile := b.Receive(ctx, "users/profile/updated")

	time.Sleep(10 * time.Millisecond)

	// Send to each topic
	b.Send(ctx, "orders/created", message.New("order-new"))
	b.Send(ctx, "orders/updated", message.New("order-upd"))
	b.Send(ctx, "users/profile/updated", message.New("user-prof"))

	// Verify isolation
	select {
	case msg := <-ordersCreated:
		if msg.Payload() != "order-new" {
			t.Errorf("Expected 'order-new', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	select {
	case msg := <-ordersUpdated:
		if msg.Payload() != "order-upd" {
			t.Errorf("Expected 'order-upd', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	select {
	case msg := <-usersProfile:
		if msg.Payload() != "user-prof" {
			t.Errorf("Expected 'user-prof', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}
}
