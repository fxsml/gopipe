package broker_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	sender := broker.NewIOSender(&buf, broker.IOConfig{})

	ctx := context.Background()
	// Payload must be JSON-encoded since Envelope.Payload is json.RawMessage
	payload, _ := json.Marshal("hello world")
	msg := message.New(payload, message.Properties{"gopipe.message.id": "msg-1"})

	if err := sender.Send(ctx, "test/topic", []*message.Message{msg}); err != nil {
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
	sender := broker.NewIOSender(&buf, broker.IOConfig{})

	ctx := context.Background()
	// Payloads must be JSON-encoded
	payload1, _ := json.Marshal(1)
	payload2, _ := json.Marshal(2)
	payload3, _ := json.Marshal(3)
	msgs := []*message.Message{
		message.New(payload1, message.Properties{}),
		message.New(payload2, message.Properties{}),
		message.New(payload3, message.Properties{}),
	}

	if err := sender.Send(ctx, "numbers", msgs); err != nil {
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
	sender := broker.NewIOSender(&buf, broker.IOConfig{})

	if err := sender.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx := context.Background()
	payload, _ := json.Marshal("msg")
	err := sender.Send(ctx, "test", []*message.Message{message.New(payload, message.Properties{})})
	if err != broker.ErrWriterClosed {
		t.Errorf("Expected ErrWriterClosed, got: %v", err)
	}
}

func TestIOReceiver_Receive(t *testing.T) {
	// First, create proper JSONL by using the sender
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()
	payload1, _ := json.Marshal("hello")
	payload2, _ := json.Marshal("world")
	sender.Send(ctx, "test", []*message.Message{message.New(payload1, message.Properties{})})
	sender.Send(ctx, "test", []*message.Message{message.New(payload2, message.Properties{})})

	// Now read it back
	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	var received []string
	for _, msg := range msgs {
		var str string
		json.Unmarshal(msg.Payload, &str)
		received = append(received, str)
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
	// Create proper JSONL using sender
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()
	payload1, _ := json.Marshal("order1")
	payload2, _ := json.Marshal("user1")
	payload3, _ := json.Marshal("order2")
	sender.Send(ctx, "orders/created", []*message.Message{message.New(payload1, message.Properties{})})
	sender.Send(ctx, "users/updated", []*message.Message{message.New(payload2, message.Properties{})})
	sender.Send(ctx, "orders/updated", []*message.Message{message.New(payload3, message.Properties{})})

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Only receive orders topics using wildcard
	msgs, err := receiver.Receive(ctx, "orders/+")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	var received []string
	for _, msg := range msgs {
		var str string
		json.Unmarshal(msg.Payload, &str)
		received = append(received, str)
	}

	if len(received) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(received))
	}
	if received[0] != "order1" || received[1] != "order2" {
		t.Errorf("Expected order messages, got %v", received)
	}
}

func TestIOReceiver_AllTopics(t *testing.T) {
	// Create proper JSONL using sender
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()
	payload1, _ := json.Marshal(1)
	payload2, _ := json.Marshal(2)
	payload3, _ := json.Marshal(3)
	sender.Send(ctx, "a", []*message.Message{message.New(payload1, message.Properties{})})
	sender.Send(ctx, "b", []*message.Message{message.New(payload2, message.Properties{})})
	sender.Send(ctx, "c", []*message.Message{message.New(payload3, message.Properties{})})

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Empty topic receives all messages
	msgs, err := receiver.Receive(ctx, "")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	var received []int
	for _, msg := range msgs {
		var num int
		json.Unmarshal(msg.Payload, &num)
		received = append(received, num)
	}

	if len(received) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(received))
	}
}

func TestIOReceiver_WithProperties(t *testing.T) {
	// Create proper JSONL using sender
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()
	payload, _ := json.Marshal("data")
	msg := message.New(payload, message.Properties{
		"gopipe.message.id": "id-123",
		"custom":            "value",
	})
	sender.Send(ctx, "test", []*message.Message{msg})

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}

	recvMsg := msgs[0]
	var str string
	json.Unmarshal(recvMsg.Payload, &str)
	if str != "data" {
		t.Errorf("Expected 'data', got '%s'", str)
	}

	if v, ok := recvMsg.Properties["custom"]; !ok || v != "value" {
		t.Errorf("Expected custom property 'value', got %v", v)
	}
}

func TestIOBroker_RoundTrip(t *testing.T) {
	// Use a pipe for bidirectional communication
	pr, pw := io.Pipe()

	sender := broker.NewIOSender(pw, broker.IOConfig{})
	receiver := broker.NewIOReceiver(pr, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var received string

	// Send message
	go func() {
		payload, _ := json.Marshal("round-trip test")
		sender.Send(ctx, "test", []*message.Message{message.New(payload, message.Properties{})})
		pw.Close() // Close to signal EOF
	}()

	// Start receiver
	wg.Add(1)
	go func() {
		defer wg.Done()
		msgs, err := receiver.Receive(ctx, "test")
		if err != nil {
			t.Logf("Receive error: %v", err)
			return
		}
		if len(msgs) > 0 {
			var str string
			json.Unmarshal(msgs[0].Payload, &str)
			received = str
		}
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
	b := broker.NewIOBroker(&buf, &buf, broker.IOConfig{})

	ctx := context.Background()

	// Send struct message - encode to JSON manually since broker uses []byte
	event := Event{Name: "test", Data: 42}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := b.Send(ctx, "events", []*message.Message{message.New(payload, message.Properties{})}); err != nil {
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
		ID       string         `json:"id"`
		Items    []string       `json:"items"`
		Total    float64        `json:"total"`
		Metadata map[string]any `json:"metadata"`
	}

	pr, pw := io.Pipe()
	b := broker.NewIOBroker(pr, pw, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var received Order

	// Send message
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
		payload, _ := json.Marshal(order)
		b.Send(ctx, "orders", []*message.Message{message.New(payload, message.Properties{})})
		pw.Close()
	}()

	// Receive message
	wg.Add(1)
	go func() {
		defer wg.Done()
		msgs, err := b.Receive(ctx, "orders")
		if err != nil {
			t.Logf("Receive error: %v", err)
			return
		}
		if len(msgs) > 0 {
			json.Unmarshal(msgs[0].Payload, &received)
		}
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
	// Create proper JSONL and inject malformed line
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()
	payload1, _ := json.Marshal("valid1")
	sender.Send(ctx, "test", []*message.Message{message.New(payload1, message.Properties{})})

	// Insert malformed line manually
	validData := buf.String()
	buf.Reset()
	buf.WriteString(validData)
	buf.WriteString("not valid json\n")

	// Add another valid message
	var buf2 bytes.Buffer
	sender2 := broker.NewIOSender(&buf2, broker.IOConfig{})
	payload2, _ := json.Marshal("valid2")
	sender2.Send(ctx, "test", []*message.Message{message.New(payload2, message.Properties{})})
	buf.WriteString(buf2.String())

	reader := strings.NewReader(buf.String())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	var received []string
	for _, msg := range msgs {
		received = append(received, string(msg.Payload))
	}

	// Should skip malformed line and receive valid messages
	if len(received) != 2 {
		t.Errorf("Expected 2 valid messages, got %d", len(received))
	}
}

func TestIOSender_ContextCancellation(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	payload, _ := json.Marshal("msg")
	err := sender.Send(ctx, "test", []*message.Message{message.New(payload, message.Properties{})})
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestIOReceiver_ContextCancellation(t *testing.T) {
	// Use a blocking reader
	pr, _ := io.Pipe()
	receiver := broker.NewIOReceiver(pr, broker.IOConfig{})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	// Receive should return with context error
	_, err := receiver.Receive(ctx, "test")
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
}

func TestIOBroker_Close(t *testing.T) {
	var buf bytes.Buffer
	b := broker.NewIOBroker(&buf, &buf, broker.IOConfig{})

	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Sender should be closed
	ctx := context.Background()
	payload, _ := json.Marshal("msg")
	if err := b.Send(ctx, "test", []*message.Message{message.New(payload, message.Properties{})}); err != broker.ErrWriterClosed {
		t.Errorf("Expected ErrWriterClosed, got: %v", err)
	}
}
