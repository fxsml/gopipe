package broker_test

import (
	"context"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func TestBroker_SendReceive(t *testing.T) {
	b := broker.NewBroker(broker.Config{})
	// No b.Close() since message.Broker does not have Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send message
	msg := message.New([]byte("hello"), message.Properties{"id": "msg-1"})
	if err := b.Send(ctx, "test/topic", []*message.Message{msg}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive messages
	received, err := b.Receive(ctx, "test/topic")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(received) == 0 {
		t.Fatal("No message received")
	}
	if string(received[0].Payload) != "hello" {
		t.Errorf("Expected payload 'hello', got '%s'", string(received[0].Payload))
	}
	if id, ok := received[0].Properties["id"]; !ok || id != "msg-1" {
		t.Errorf("Expected ID 'msg-1', got '%s'", id)
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := broker.NewBroker(broker.Config{BufferSize: 10})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send message
	msg := message.New([]byte("42"), message.Properties{})
	if err := b.Send(ctx, "events", []*message.Message{msg}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive messages from multiple subscribers
	received1, err1 := b.Receive(ctx, "events")
	received2, err2 := b.Receive(ctx, "events")
	received3, err3 := b.Receive(ctx, "events")

	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("Receive failed: %v %v %v", err1, err2, err3)
	}
	if len(received1) == 0 || len(received2) == 0 || len(received3) == 0 {
		t.Fatal("One or more subscribers did not receive message")
	}
	if string(received1[0].Payload) != "42" || string(received2[0].Payload) != "42" || string(received3[0].Payload) != "42" {
		t.Errorf("Expected payload '42' for all subscribers")
	}
}

func TestBroker_MultipleTopics(t *testing.T) {
	b := broker.NewBroker(broker.Config{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send to different topics
	msg1 := message.New([]byte("order-123"), message.Properties{})
	msg2 := message.New([]byte("user-456"), message.Properties{})
	b.Send(ctx, "orders/created", []*message.Message{msg1})
	b.Send(ctx, "users/updated", []*message.Message{msg2})

	// Receive from different topics
	ordersRecv, err1 := b.Receive(ctx, "orders/created")
	usersRecv, err2 := b.Receive(ctx, "users/updated")
	if err1 != nil || err2 != nil {
		t.Fatalf("Receive failed: %v %v", err1, err2)
	}
	if len(ordersRecv) == 0 || len(usersRecv) == 0 {
		t.Fatal("No message received for one or both topics")
	}
	if string(ordersRecv[0].Payload) != "order-123" {
		t.Errorf("Orders: expected 'order-123', got '%s'", string(ordersRecv[0].Payload))
	}
	if string(usersRecv[0].Payload) != "user-456" {
		t.Errorf("Users: expected 'user-456', got '%s'", string(usersRecv[0].Payload))
	}
}

func TestBroker_TopicsCreatedOnFly(t *testing.T) {
	b := broker.NewBroker(broker.Config{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send to non-existent topic (should create it)
	msg1 := message.New([]byte("first"), message.Properties{})
	err := b.Send(ctx, "new/topic/path", []*message.Message{msg1})
	if err != nil {
		t.Fatalf("Send to new topic failed: %v", err)
	}

	// Send again
	msg2 := message.New([]byte("second"), message.Properties{})
	err = b.Send(ctx, "new/topic/path", []*message.Message{msg2})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive
	recv, err := b.Receive(ctx, "new/topic/path")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	found := false
	for _, m := range recv {
		if string(m.Payload) == "second" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'second' in received messages")
	}
}

// Skipped: TestBroker_Close (lifecycle not supported by message.Broker interface)

// Skipped: TestBroker_ContextCancellation (channel-based receive not supported)

func TestBroker_SendTimeout(t *testing.T) {
	b := broker.NewBroker(broker.Config{SendTimeout: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := message.New([]byte("msg"), message.Properties{})
	err := b.Send(ctx, "test", []*message.Message{msg})
	if err == nil {
		t.Error("Expected error on send with canceled context")
	}
}

func TestNewSender(t *testing.T) {
	b := broker.NewBroker(broker.Config{})
	sender := broker.NewSender(b)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msg := message.New([]byte("hello"), message.Properties{})
	err := sender.Send(ctx, "test", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send via Sender interface failed: %v", err)
	}
}

func TestNewReceiver(t *testing.T) {
	b := broker.NewBroker(broker.Config{})
	receiver := broker.NewReceiver(b)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msg := message.New([]byte("hello"), message.Properties{})
	b.Send(ctx, "test", []*message.Message{msg})

	recv, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(recv) == 0 || string(recv[0].Payload) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(recv[0].Payload))
	}
}

func TestBroker_BatchSend(t *testing.T) {
	b := broker.NewBroker(broker.Config{BufferSize: 100})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send multiple messages at once
	msgs := []*message.Message{
		message.New([]byte("one"), message.Properties{}),
		message.New([]byte("two"), message.Properties{}),
		message.New([]byte("three"), message.Properties{}),
	}
	if err := b.Send(ctx, "batch", msgs); err != nil {
		t.Fatalf("Batch send failed: %v", err)
	}

	// Receive all messages
	received, err := b.Receive(ctx, "batch")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(received) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(received))
	}
	expected := []string{"one", "two", "three"}
	for i, msg := range received {
		if string(msg.Payload) != expected[i] {
			t.Errorf("Expected '%s', got '%s'", expected[i], string(msg.Payload))
		}
	}
}

func TestBroker_HierarchicalTopics(t *testing.T) {
	b := broker.NewBroker(broker.Config{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send to each topic
	msg1 := message.New([]byte("order-new"), message.Properties{})
	msg2 := message.New([]byte("order-upd"), message.Properties{})
	msg3 := message.New([]byte("user-prof"), message.Properties{})
	b.Send(ctx, "orders/created", []*message.Message{msg1})
	b.Send(ctx, "orders/updated", []*message.Message{msg2})
	b.Send(ctx, "users/profile/updated", []*message.Message{msg3})

	// Receive from each topic
	ordersCreated, err1 := b.Receive(ctx, "orders/created")
	ordersUpdated, err2 := b.Receive(ctx, "orders/updated")
	usersProfile, err3 := b.Receive(ctx, "users/profile/updated")
	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("Receive failed: %v %v %v", err1, err2, err3)
	}
	if len(ordersCreated) == 0 || string(ordersCreated[0].Payload) != "order-new" {
		t.Errorf("Expected 'order-new', got '%s'", string(ordersCreated[0].Payload))
	}
	if len(ordersUpdated) == 0 || string(ordersUpdated[0].Payload) != "order-upd" {
		t.Errorf("Expected 'order-upd', got '%s'", string(ordersUpdated[0].Payload))
	}
	if len(usersProfile) == 0 || string(usersProfile[0].Payload) != "user-prof" {
		t.Errorf("Expected 'user-prof', got '%s'", string(usersProfile[0].Payload))
	}
}
