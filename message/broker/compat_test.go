package broker_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

// TestIOCompatibility ensures IO sender and receiver are compatible:
// - Send writes messages that Receive can read
// - Messages stream correctly through io.Reader/Writer
// - Works with pipes, files, and any io.Reader/Writer
func TestIOCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		messages []*message.Message
		topic    string
	}{
		{
			name: "single message",
			messages: []*message.Message{
				message.New([]byte(`{"hello":"world"}`), message.Attributes{"id": "msg-1"}),
			},
			topic: "test/single",
		},
		{
			name: "multiple messages",
			messages: []*message.Message{
				message.New([]byte(`"first"`), message.Attributes{"seq": "1"}),
				message.New([]byte(`"second"`), message.Attributes{"seq": "2"}),
				message.New([]byte(`"third"`), message.Attributes{"seq": "3"}),
			},
			topic: "test/multiple",
		},
		{
			name: "different topics",
			messages: []*message.Message{
				message.New([]byte(`"order created"`), message.Attributes{"type": "order"}),
				message.New([]byte(`"user registered"`), message.Attributes{"type": "user"}),
			},
			topic: "events/all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a buffer to simulate a file or stream
			var buf bytes.Buffer

			// Create sender and write messages
			sender := broker.NewIOSender(&buf, broker.IOConfig{})
			ctx := context.Background()

			for _, msg := range tt.messages {
				if err := sender.Send(ctx, tt.topic, []*message.Message{msg}); err != nil {
					t.Fatalf("Send failed: %v", err)
				}
			}

			// Create receiver and read messages
			reader := bytes.NewReader(buf.Bytes())
			receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			received, err := receiver.Receive(ctx, tt.topic)
			if err != nil && err != context.Canceled {
				t.Fatalf("Receive failed: %v", err)
			}

			// Verify messages
			if len(received) != len(tt.messages) {
				t.Errorf("Expected %d messages, got %d", len(tt.messages), len(received))
			}

			for i, msg := range received {
				if i >= len(tt.messages) {
					break
				}
				if !bytes.Equal(msg.Data, tt.messages[i].Data) {
					t.Errorf("Message %d payload mismatch: got %s, want %s",
						i, msg.Data, tt.messages[i].Data)
				}
			}
		})
	}
}

// TestIOPipeCompatibility tests IO b with pipes for IPC
func TestIOPipeCompatibility(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	b := broker.NewIOBroker(pr, pw, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send and receive in goroutines to avoid deadlock
	sent := make(chan struct{})
	received := make(chan []*message.Message, 1)

	// Receiver goroutine
	go func() {
		msgs, err := b.Receive(ctx, "pipe/test")
		if err != nil && err != context.Canceled && err != io.EOF {
			t.Errorf("Receive error: %v", err)
		}
		received <- msgs
	}()

	// Sender goroutine
	go func() {
		time.Sleep(50 * time.Millisecond) // Let receiver start
		msg := message.New([]byte(`"pipe message"`), message.Attributes{"via": "pipe"})
		if err := b.Send(ctx, "pipe/test", []*message.Message{msg}); err != nil {
			t.Errorf("Send error: %v", err)
		}
		pw.Close() // Close to signal EOF
		close(sent)
	}()

	// Wait for completion
	<-sent
	msgs := <-received

	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}
	if len(msgs) > 0 && !bytes.Equal(msgs[0].Data, []byte(`"pipe message"`)) {
		t.Errorf("Payload mismatch: got %s", msgs[0].Data)
	}
}

// TestHTTPCompatibility ensures HTTP sender and receiver are compatible:
// - Sender POSTs messages that Receiver can handle
// - Headers are correctly encoded/decoded
// - Properties are preserved through HTTP transport
func TestHTTPCompatibility(t *testing.T) {
	// Create HTTP receiver
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	// Start test HTTP server with receiver handler
	server := httptest.NewServer(receiver.Handler())
	defer server.Close()

	// Create HTTP sender pointing to test server
	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		SendTimeout: time.Second,
	})

	// Send messages
	ctx := context.Background()
	messages := []*message.Message{
		message.New([]byte(`{"event": "order.created"}`), message.Attributes{
			"id":       "evt-001",
			"priority": "high",
		}),
		message.New([]byte(`{"event": "user.registered"}`), message.Attributes{
			"id":     "evt-002",
			"source": "api",
		}),
	}

	for _, msg := range messages {
		if err := sender.Send(ctx, "webhooks/events", []*message.Message{msg}); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
	}

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Receive messages
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	received, err := receiver.Receive(ctx, "webhooks/events")
	if err != nil && err != context.Canceled {
		t.Fatalf("Receive failed: %v", err)
	}

	// Verify messages
	if len(received) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(received))
	}

	for i, msg := range received {
		if i >= len(messages) {
			break
		}
		if !bytes.Equal(msg.Data, messages[i].Data) {
			t.Errorf("Message %d payload mismatch: got %s, want %s",
				i, msg.Data, messages[i].Data)
		}
		// Note: HTTP properties are converted to strings during transport,
		// so we just verify they exist
		if len(msg.Attributes) == 0 {
			t.Errorf("Message %d has no properties", i)
		}
	}
}

// TestHTTPTopicRouting tests HTTP sender/receiver with different topics
func TestHTTPTopicRouting(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	server := httptest.NewServer(receiver.Handler())
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})
	ctx := context.Background()

	// Send to different topics
	topics := []string{
		"orders/created",
		"orders/updated",
		"users/registered",
	}

	for _, topic := range topics {
		msg := message.New([]byte(topic), message.Attributes{"topic": topic})
		if err := sender.Send(ctx, topic, []*message.Message{msg}); err != nil {
			t.Fatalf("Send to %s failed: %v", topic, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Receive from specific topic (exact match only)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	received, err := receiver.Receive(ctx, "orders/created")
	if err != nil && err != context.Canceled {
		t.Fatalf("Receive failed: %v", err)
	}

	// Should receive 1 message (exact match)
	if len(received) != 1 {
		t.Errorf("Expected 1 orders/created message, got %d", len(received))
	}
}

// TestIOHTTPRoundTrip tests that messages can be sent via IO, transformed, and sent via HTTP
func TestIOHTTPRoundTrip(t *testing.T) {
	// Step 1: Write messages to IO
	var buf bytes.Buffer
	ioSender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()

	originalMsg := message.New([]byte(`"test message"`), message.Attributes{
		"source": "io",
		"id":     "123",
	})

	if err := ioSender.Send(ctx, "test/topic", []*message.Message{originalMsg}); err != nil {
		t.Fatalf("IO Send failed: %v", err)
	}

	// Step 2: Read from IO
	reader := bytes.NewReader(buf.Bytes())
	ioReceiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ioMessages, err := ioReceiver.Receive(ctx, "test/topic")
	if err != nil && err != context.Canceled {
		t.Fatalf("IO Receive failed: %v", err)
	}

	if len(ioMessages) != 1 {
		t.Fatalf("Expected 1 message from IO, got %d", len(ioMessages))
	}

	// Step 3: Send via HTTP
	httpReceiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer httpReceiver.Close()

	server := httptest.NewServer(httpReceiver.Handler())
	defer server.Close()

	httpSender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})

	// Transform message: add new property
	transformedMsg := message.New(ioMessages[0].Data, message.Attributes{
		"source":      "http",
		"original-id": ioMessages[0].Attributes["id"],
	})

	if err := httpSender.Send(ctx, "test/topic", []*message.Message{transformedMsg}); err != nil {
		t.Fatalf("HTTP Send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Step 4: Receive via HTTP
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	httpMessages, err := httpReceiver.Receive(ctx, "test/topic")
	if err != nil && err != context.Canceled {
		t.Fatalf("HTTP Receive failed: %v", err)
	}

	if len(httpMessages) != 1 {
		t.Fatalf("Expected 1 message from HTTP, got %d", len(httpMessages))
	}

	// Verify round-trip
	if !bytes.Equal(httpMessages[0].Data, originalMsg.Data) {
		t.Errorf("Payload mismatch after round-trip: got %s, want %s",
			httpMessages[0].Data, originalMsg.Data)
	}
}

// TestIOTopicFiltering verifies topic filtering behavior:
// - Send writes ALL messages (topic preserved in CloudEvent subject)
// - Receive with empty topic returns all messages
// - Receive with specific topic filters by exact match
func TestIOTopicFiltering(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()

	// Write messages with different topics
	msgs := map[string]*message.Message{
		"orders.created":   message.New([]byte(`"order1"`), nil),
		"orders.shipped":   message.New([]byte(`"order2"`), nil),
		"payments.charged": message.New([]byte(`"payment1"`), nil),
	}

	for topic, msg := range msgs {
		if err := sender.Send(ctx, topic, []*message.Message{msg}); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
	}

	t.Run("empty topic returns all messages", func(t *testing.T) {
		reader := bytes.NewReader(buf.Bytes())
		receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		received, err := receiver.Receive(ctx, "") // empty = all
		if err != nil && err != context.Canceled {
			t.Fatalf("Receive failed: %v", err)
		}

		if len(received) != 3 {
			t.Errorf("Expected 3 messages with empty topic, got %d", len(received))
		}
	})

	t.Run("specific topic filters by exact match", func(t *testing.T) {
		reader := bytes.NewReader(buf.Bytes())
		receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		received, err := receiver.Receive(ctx, "orders.created")
		if err != nil && err != context.Canceled {
			t.Fatalf("Receive failed: %v", err)
		}

		if len(received) != 1 {
			t.Errorf("Expected 1 message for orders.created, got %d", len(received))
		}
		if len(received) > 0 && !bytes.Equal(received[0].Data, []byte(`"order1"`)) {
			t.Errorf("Got wrong message: %s", received[0].Data)
		}
	})

	t.Run("non-matching topic returns empty", func(t *testing.T) {
		reader := bytes.NewReader(buf.Bytes())
		receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		received, err := receiver.Receive(ctx, "nonexistent.topic")
		if err != nil && err != context.Canceled {
			t.Fatalf("Receive failed: %v", err)
		}

		if len(received) != 0 {
			t.Errorf("Expected 0 messages for nonexistent topic, got %d", len(received))
		}
	})
}

// TestIOTopicPreservation verifies that topics are preserved through serialization
// so they can be used for routing when republishing to a real broker.
func TestIOTopicPreservation(t *testing.T) {
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})
	ctx := context.Background()

	originalTopic := "orders.created"
	msg := message.New([]byte(`"test"`), nil)

	if err := sender.Send(ctx, originalTopic, []*message.Message{msg}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Read back and verify topic is preserved in attributes
	reader := bytes.NewReader(buf.Bytes())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	received, err := receiver.Receive(ctx, "")
	if err != nil && err != context.Canceled {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(received))
	}

	// Topic should be preserved in Topic attribute
	topic, ok := received[0].Attributes.Topic()
	if !ok {
		t.Fatal("Topic attribute not preserved")
	}
	if topic != originalTopic {
		t.Errorf("Topic not preserved: got %q, want %q", topic, originalTopic)
	}
}
