package broker_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func TestIOSender_Send(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})

	ctx := context.Background()
	msg := message.New("hello world", message.WithID[string]("msg-1"))

	if err := sender.Send(ctx, "test/topic", msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"topic":"test/topic"`) {
		t.Errorf("Expected topic in output, got: %s", output)
	}
	if !strings.Contains(output, `"hello world"`) {
		t.Errorf("Expected payload in output, got: %s", output)
	}
	if !strings.Contains(output, `gopipe.message.id`) {
		t.Errorf("Expected message ID property in output, got: %s", output)
	}
}

func TestIOSender_BatchSend(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender[int](&buf, broker.IOConfig{})

	ctx := context.Background()
	msgs := []*message.Message[int]{
		message.New(1),
		message.New(2),
		message.New(3),
	}

	if err := sender.Send(ctx, "numbers", msgs...); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Should have 3 newline-delimited JSON lines
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
}

func TestIOSender_Close(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})

	if err := sender.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx := context.Background()
	err := sender.Send(ctx, "test", message.New("msg"))
	if err != broker.ErrWriterClosed {
		t.Errorf("Expected ErrWriterClosed, got: %v", err)
	}
}

func TestIOReceiver_Receive(t *testing.T) {
	// First, create proper NDJSON by using the sender
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})
	ctx := context.Background()
	sender.Send(ctx, "test", message.New("hello"))
	sender.Send(ctx, "test", message.New("world"))

	// Now read it back
	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver[string](reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := receiver.Receive(ctx, "test")

	var received []string
	for msg := range msgs {
		received = append(received, msg.Payload())
	}

	if len(received) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(received))
	}
	if received[0] != "hello" {
		t.Errorf("Expected 'hello', got '%s'", received[0])
	}
	if received[1] != "world" {
		t.Errorf("Expected 'world', got '%s'", received[1])
	}
}

func TestIOReceiver_TopicFilter(t *testing.T) {
	// Create proper NDJSON using sender
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})
	ctx := context.Background()
	sender.Send(ctx, "orders/created", message.New("order1"))
	sender.Send(ctx, "users/updated", message.New("user1"))
	sender.Send(ctx, "orders/updated", message.New("order2"))

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver[string](reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Only receive orders topics using wildcard
	msgs := receiver.Receive(ctx, "orders/+")

	var received []string
	for msg := range msgs {
		received = append(received, msg.Payload())
	}

	if len(received) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(received))
	}
	if received[0] != "order1" || received[1] != "order2" {
		t.Errorf("Expected order messages, got %v", received)
	}
}

func TestIOReceiver_AllTopics(t *testing.T) {
	// Create proper NDJSON using sender
	var buf bytes.Buffer
	sender := broker.NewIOSender[int](&buf, broker.IOConfig{})
	ctx := context.Background()
	sender.Send(ctx, "a", message.New(1))
	sender.Send(ctx, "b", message.New(2))
	sender.Send(ctx, "c", message.New(3))

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver[int](reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Empty topic receives all messages
	msgs := receiver.Receive(ctx, "")

	var received []int
	for msg := range msgs {
		received = append(received, msg.Payload())
	}

	if len(received) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(received))
	}
}

func TestIOReceiver_WithProperties(t *testing.T) {
	// Create proper NDJSON using sender
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})
	ctx := context.Background()
	sender.Send(ctx, "test", message.New("data",
		message.WithID[string]("id-123"),
		message.WithProperty[string]("custom", "value"),
	))

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver[string](reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := receiver.Receive(ctx, "test")

	msg := <-msgs
	if msg == nil {
		t.Fatal("Expected message")
	}

	if msg.Payload() != "data" {
		t.Errorf("Expected 'data', got '%s'", msg.Payload())
	}

	if v, ok := msg.Properties().Get("custom"); !ok || v != "value" {
		t.Errorf("Expected custom property 'value', got %v", v)
	}
}

func TestIOBroker_RoundTrip(t *testing.T) {
	// Use a pipe for bidirectional communication
	pr, pw := io.Pipe()

	sender := broker.NewIOSender[string](pw, broker.IOConfig{})
	receiver := broker.NewIOReceiver[string](pr, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start receiver
	msgs := receiver.Receive(ctx, "test")

	var wg sync.WaitGroup
	var received string

	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range msgs {
			received = msg.Payload()
			return // Only expect one message
		}
	}()

	// Send message
	go func() {
		sender.Send(ctx, "test", message.New("round-trip test"))
		pw.Close() // Close to signal EOF
	}()

	wg.Wait()

	if received != "round-trip test" {
		t.Errorf("Expected 'round-trip test', got '%s'", received)
	}
}

func TestIOBroker_Struct(t *testing.T) {
	type Event struct {
		Name string `json:"name"`
		Data int    `json:"data"`
	}

	var buf bytes.Buffer
	b := broker.NewIOBroker[Event](&buf, &buf, broker.IOConfig{})

	ctx := context.Background()

	// Send struct message
	event := Event{Name: "test", Data: 42}
	if err := b.Send(ctx, "events", message.New(event)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name":"test"`) {
		t.Errorf("Expected struct fields in output, got: %s", output)
	}
	if !strings.Contains(output, `"data":42`) {
		t.Errorf("Expected struct fields in output, got: %s", output)
	}
}

func TestIOBroker_ComplexTypes(t *testing.T) {
	type Order struct {
		ID       string   `json:"id"`
		Items    []string `json:"items"`
		Total    float64  `json:"total"`
		Metadata map[string]any `json:"metadata"`
	}

	pr, pw := io.Pipe()
	b := broker.NewIOBroker[Order](pr, pw, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := b.Receive(ctx, "orders")

	var wg sync.WaitGroup
	var received Order

	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range msgs {
			received = msg.Payload()
			return
		}
	}()

	go func() {
		order := Order{
			ID:    "ord-123",
			Items: []string{"item1", "item2"},
			Total: 99.99,
			Metadata: map[string]any{
				"priority": "high",
				"count":    float64(2),
			},
		}
		b.Send(ctx, "orders", message.New(order))
		pw.Close()
	}()

	wg.Wait()

	if received.ID != "ord-123" {
		t.Errorf("Expected ID 'ord-123', got '%s'", received.ID)
	}
	if len(received.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(received.Items))
	}
	if received.Total != 99.99 {
		t.Errorf("Expected total 99.99, got %f", received.Total)
	}
}

func TestIOReceiver_MalformedJSON(t *testing.T) {
	// Create proper NDJSON and inject malformed line
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})
	ctx := context.Background()
	sender.Send(ctx, "test", message.New("valid1"))

	// Insert malformed line manually
	validData := buf.String()
	buf.Reset()
	buf.WriteString(validData)
	buf.WriteString("not valid json\n")

	// Add another valid message
	var buf2 bytes.Buffer
	sender2 := broker.NewIOSender[string](&buf2, broker.IOConfig{})
	sender2.Send(ctx, "test", message.New("valid2"))
	buf.WriteString(buf2.String())

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver[string](reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := receiver.Receive(ctx, "test")

	var received []string
	for msg := range msgs {
		received = append(received, msg.Payload())
	}

	// Should skip malformed line and receive valid messages
	if len(received) != 2 {
		t.Errorf("Expected 2 valid messages, got %d", len(received))
	}
}

func TestIOSender_ContextCancellation(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender[string](&buf, broker.IOConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := sender.Send(ctx, "test", message.New("msg"))
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestIOReceiver_ContextCancellation(t *testing.T) {
	// Use a blocking reader
	pr, _ := io.Pipe()
	receiver := broker.NewIOReceiver[string](pr, broker.IOConfig{})

	ctx, cancel := context.WithCancel(context.Background())

	msgs := receiver.Receive(ctx, "test")

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-msgs:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for channel close")
	}
}

func TestIOBroker_Close(t *testing.T) {
	var buf bytes.Buffer
	b := broker.NewIOBroker[string](&buf, &buf, broker.IOConfig{})

	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Sender should be closed
	ctx := context.Background()
	if err := b.Send(ctx, "test", message.New("msg")); err != broker.ErrWriterClosed {
		t.Errorf("Expected ErrWriterClosed, got: %v", err)
	}
}
