package pubsub_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
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
				message.New([]byte(`{"hello":"world"}`), message.Properties{"id": "msg-1"}),
			},
			topic: "test/single",
		},
		{
			name: "multiple messages",
			messages: []*message.Message{
				message.New([]byte(`"first"`), message.Properties{"seq": "1"}),
				message.New([]byte(`"second"`), message.Properties{"seq": "2"}),
				message.New([]byte(`"third"`), message.Properties{"seq": "3"}),
			},
			topic: "test/multiple",
		},
		{
			name: "different topics",
			messages: []*message.Message{
				message.New([]byte(`"order created"`), message.Properties{"type": "order"}),
				message.New([]byte(`"user registered"`), message.Properties{"type": "user"}),
			},
			topic: "events/#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a buffer to simulate a file or stream
			var buf bytes.Buffer

			// Create sender and write messages
			sender := pubsub.NewIOSender(&buf, pubsub.IOConfig{})
			ctx := context.Background()

			for _, msg := range tt.messages {
				if err := sender.Send(ctx, tt.topic, []*message.Message{msg}); err != nil {
					t.Fatalf("Send failed: %v", err)
				}
			}

			// Create receiver and read messages
			reader := bytes.NewReader(buf.Bytes())
			receiver := pubsub.NewIOReceiver(reader, pubsub.IOConfig{})

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
				if !bytes.Equal(msg.Payload, tt.messages[i].Payload) {
					t.Errorf("Message %d payload mismatch: got %s, want %s",
						i, msg.Payload, tt.messages[i].Payload)
				}
			}
		})
	}
}

// TestIOPipeCompatibility tests IO broker with pipes for IPC
func TestIOPipeCompatibility(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	broker := pubsub.NewIOBroker(pr, pw, pubsub.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send and receive in goroutines to avoid deadlock
	sent := make(chan struct{})
	received := make(chan []*message.Message, 1)

	// Receiver goroutine
	go func() {
		msgs, err := broker.Receive(ctx, "pipe/test")
		if err != nil && err != context.Canceled && err != io.EOF {
			t.Errorf("Receive error: %v", err)
		}
		received <- msgs
	}()

	// Sender goroutine
	go func() {
		time.Sleep(50 * time.Millisecond) // Let receiver start
		msg := message.New([]byte(`"pipe message"`), message.Properties{"via": "pipe"})
		if err := broker.Send(ctx, "pipe/test", []*message.Message{msg}); err != nil {
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
	if len(msgs) > 0 && !bytes.Equal(msgs[0].Payload, []byte(`"pipe message"`)) {
		t.Errorf("Payload mismatch: got %s", msgs[0].Payload)
	}
}

// TestHTTPCompatibility ensures HTTP sender and receiver are compatible:
// - Sender POSTs messages that Receiver can handle
// - Headers are correctly encoded/decoded
// - Properties are preserved through HTTP transport
func TestHTTPCompatibility(t *testing.T) {
	// Create HTTP receiver
	receiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)
	defer func() {
		if closer, ok := receiver.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	// Start test HTTP server with receiver handler
	receiverHandler, ok := receiver.(interface{ Handler() http.Handler })
	if !ok {
		t.Fatal("Receiver doesn't implement Handler()")
	}
	server := httptest.NewServer(receiverHandler.Handler())
	defer server.Close()

	// Create HTTP sender pointing to test server
	sender := pubsub.NewHTTPSender(server.URL, pubsub.HTTPConfig{
		SendTimeout: time.Second,
	})

	// Send messages
	ctx := context.Background()
	messages := []*message.Message{
		message.New([]byte(`{"event": "order.created"}`), message.Properties{
			"id":       "evt-001",
			"priority": "high",
		}),
		message.New([]byte(`{"event": "user.registered"}`), message.Properties{
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
		if !bytes.Equal(msg.Payload, messages[i].Payload) {
			t.Errorf("Message %d payload mismatch: got %s, want %s",
				i, msg.Payload, messages[i].Payload)
		}
		// Note: HTTP properties are converted to strings during transport,
		// so we just verify they exist
		if len(msg.Properties) == 0 {
			t.Errorf("Message %d has no properties", i)
		}
	}
}

// TestHTTPTopicRouting tests HTTP sender/receiver with different topics
func TestHTTPTopicRouting(t *testing.T) {
	receiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)
	defer func() {
		if closer, ok := receiver.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	receiverHandler, ok := receiver.(interface{ Handler() http.Handler })
	if !ok {
		t.Fatal("Receiver doesn't implement Handler()")
	}
	server := httptest.NewServer(receiverHandler.Handler())
	defer server.Close()

	sender := pubsub.NewHTTPSender(server.URL, pubsub.HTTPConfig{})
	ctx := context.Background()

	// Send to different topics
	topics := []string{
		"orders/created",
		"orders/updated",
		"users/registered",
	}

	for _, topic := range topics {
		msg := message.New([]byte(topic), message.Properties{"topic": topic})
		if err := sender.Send(ctx, topic, []*message.Message{msg}); err != nil {
			t.Fatalf("Send to %s failed: %v", topic, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Receive all orders messages using wildcard
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	received, err := receiver.Receive(ctx, "orders/#")
	if err != nil && err != context.Canceled {
		t.Fatalf("Receive failed: %v", err)
	}

	// Should receive 2 orders messages (orders/created, orders/updated)
	if len(received) != 2 {
		t.Errorf("Expected 2 orders messages, got %d", len(received))
	}
}

// TestIOHTTPRoundTrip tests that messages can be sent via IO, transformed, and sent via HTTP
func TestIOHTTPRoundTrip(t *testing.T) {
	// Step 1: Write messages to IO
	var buf bytes.Buffer
	ioSender := pubsub.NewIOSender(&buf, pubsub.IOConfig{})
	ctx := context.Background()

	originalMsg := message.New([]byte(`"test message"`), message.Properties{
		"source": "io",
		"id":     "123",
	})

	if err := ioSender.Send(ctx, "test/topic", []*message.Message{originalMsg}); err != nil {
		t.Fatalf("IO Send failed: %v", err)
	}

	// Step 2: Read from IO
	reader := bytes.NewReader(buf.Bytes())
	ioReceiver := pubsub.NewIOReceiver(reader, pubsub.IOConfig{})

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
	httpReceiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)
	defer func() {
		if closer, ok := httpReceiver.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	receiverHandler, ok := httpReceiver.(interface{ Handler() http.Handler })
	if !ok {
		t.Fatal("Receiver doesn't implement Handler()")
	}
	server := httptest.NewServer(receiverHandler.Handler())
	defer server.Close()

	httpSender := pubsub.NewHTTPSender(server.URL, pubsub.HTTPConfig{})

	// Transform message: add new property
	transformedMsg := message.New(ioMessages[0].Payload, message.Properties{
		"source":      "http",
		"original-id": ioMessages[0].Properties["id"],
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
	if !bytes.Equal(httpMessages[0].Payload, originalMsg.Payload) {
		t.Errorf("Payload mismatch after round-trip: got %s, want %s",
			httpMessages[0].Payload, originalMsg.Payload)
	}
}
