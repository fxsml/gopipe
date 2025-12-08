package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func main() {
	fmt.Println("=== In-Memory Broker Example ===")
	fmt.Println()

	// Create broker with configuration
	b := broker.NewBroker(broker.Config{
		CloseTimeout: 5 * time.Second,
		SendTimeout:  time.Second,
		BufferSize:   50,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Example 1: Basic pub/sub with separate sender/receiver
	fmt.Println("--- Example 1: Basic Pub/Sub ---")
	basicPubSub(ctx, b, &wg)

	// Example 2: Multiple subscribers on same topic
	fmt.Println("\n--- Example 2: Multiple Subscribers ---")
	multipleSubscribers(ctx, b, &wg)

	// Example 3: Hierarchical topics
	fmt.Println("\n--- Example 3: Hierarchical Topics ---")
	hierarchicalTopics(ctx, b, &wg)

	// Example 4: Integration with gopipe processors
	fmt.Println("\n--- Example 4: Gopipe Integration ---")
	gopipeIntegration(ctx, b, &wg)

	wg.Wait()

	// Example 5: IO Broker with file/stream support
	fmt.Println("\n--- Example 5: IO Broker (JSON Streaming) ---")
	ioBrokerExample()

	// Example 6: IO Broker with pipe for IPC
	fmt.Println("\n--- Example 6: IO Broker (Pipe IPC) ---")
	ioBrokerPipeExample()

	// Example 7: HTTP Broker (webhook style)
	fmt.Println("\n--- Example 7: HTTP Broker (Webhook) ---")
	httpBrokerExample()

	fmt.Println("\n=== All examples completed ===")
}

func basicPubSub(ctx context.Context, b message.Broker, wg *sync.WaitGroup) {
	// Get sender and receiver interfaces
	sender := broker.NewSender(b)
	receiver := broker.NewReceiver(b)

	// Start subscriber - returns (messages, error)
	msgs, err := receiver.Receive(ctx, "greetings")
	if err != nil {
		fmt.Printf("Error receiving: %v\n", err)
		return
	}

	wg.Add(1)
	go func(messages []*message.Message) {
		defer wg.Done()
		for _, msg := range messages {
			fmt.Printf("  Received: %s\n", msg.Payload)
		}
	}(msgs)

	// Give subscriber time to connect
	time.Sleep(10 * time.Millisecond)

	// Publish messages
	sender.Send(ctx, "greetings", []*message.Message{message.New([]byte("Hello, World!"), message.Properties{})})
	sender.Send(ctx, "greetings", []*message.Message{message.New([]byte("Welcome to gopipe!"), message.Properties{})})

	time.Sleep(50 * time.Millisecond)
}

func multipleSubscribers(ctx context.Context, b message.Broker, wg *sync.WaitGroup) {
	// Create multiple subscribers for the same topic
	msgs1, _ := b.Receive(ctx, "events")
	msgs2, _ := b.Receive(ctx, "events")

	// Process messages from both subscribers
	for i, msgs := range [][]*message.Message{msgs1, msgs2} {
		wg.Add(1)
		go func(id int, messages []*message.Message) {
			defer wg.Done()
			for _, msg := range messages {
				fmt.Printf("  Subscriber %d received: %s\n", id, msg.Payload)
			}
		}(i+1, msgs)
	}

	time.Sleep(10 * time.Millisecond)

	// Send event - both subscribers will receive it
	b.Send(ctx, "events", []*message.Message{message.New([]byte("Important event occurred!"), message.Properties{})})

	time.Sleep(50 * time.Millisecond)
}

func hierarchicalTopics(ctx context.Context, b message.Broker, wg *sync.WaitGroup) {
	// Subscribe to different hierarchical topics
	ordersCreated, _ := b.Receive(ctx, "orders/created")
	ordersUpdated, _ := b.Receive(ctx, "orders/updated")
	usersProfile, _ := b.Receive(ctx, "users/profile/updated")

	// Combine all messages (broker returns slices, not channels)
	merged := append(append(ordersCreated, ordersUpdated...), usersProfile...)

	wg.Add(1)
	go func(messages []*message.Message) {
		defer wg.Done()
		for _, msg := range messages {
			fmt.Printf("  Received on merged channel: %s\n", msg.Payload)
		}
	}(merged)

	time.Sleep(10 * time.Millisecond)

	// Send to different topics
	b.Send(ctx, "orders/created", []*message.Message{message.New([]byte("Order #123 created"), message.Properties{})})
	b.Send(ctx, "orders/updated", []*message.Message{message.New([]byte("Order #456 shipped"), message.Properties{})})
	b.Send(ctx, "users/profile/updated", []*message.Message{message.New([]byte("User john updated profile"), message.Properties{})})

	time.Sleep(50 * time.Millisecond)
}

func gopipeIntegration(ctx context.Context, b message.Broker, wg *sync.WaitGroup) {
	// Note: Broker now returns slices, not channels, so direct pipe integration needs adjustment
	// For this example, we'll demonstrate receiving and processing messages
	incoming, _ := b.Receive(ctx, "input")

	// Process messages (broker returns slices, not channels)
	processed := make([]*message.Message, 0, len(incoming))
	for _, msg := range incoming {
		// Transform: add processing prefix
		transformed := fmt.Sprintf("[PROCESSED] %s", msg.Payload)
		processed = append(processed, message.New([]byte(transformed), msg.Properties))
	}

	// Forward processed messages to output topic
	if len(processed) > 0 {
		b.Send(ctx, "output", processed)
	}

	// Subscribe to output topic
	output, _ := b.Receive(ctx, "output")
	wg.Add(1)
	go func(messages []*message.Message) {
		defer wg.Done()
		for _, msg := range messages {
			fmt.Printf("  Final output topic: %s\n", msg.Payload)
		}
	}(output)

	time.Sleep(10 * time.Millisecond)

	// Send messages through the pipeline
	b.Send(ctx, "input", []*message.Message{message.New([]byte("message one"), message.Properties{})})
	b.Send(ctx, "input", []*message.Message{message.New([]byte("message two"), message.Properties{})})

	time.Sleep(100 * time.Millisecond)
}

func ioBrokerExample() {
	// IO broker writes messages as JSON Lines (JSONL)
	// This can be used for file-based persistence or log streaming

	// Write messages to a buffer (could be a file)
	var buf bytes.Buffer
	sender := broker.NewIOSender(&buf, broker.IOConfig{})

	ctx := context.Background()

	// Send messages with properties
	msg1 := message.New([]byte("Application started"), message.Properties{"id": "log-001", "level": "info"})
	sender.Send(ctx, "logs/app", []*message.Message{msg1})
	
	msg2 := message.New([]byte("Processing request"), message.Properties{"id": "log-002", "level": "debug"})
	sender.Send(ctx, "logs/app", []*message.Message{msg2})
	
	msg3 := message.New([]byte("Connection failed"), message.Properties{"id": "log-003", "level": "error"})
	sender.Send(ctx, "logs/error", []*message.Message{msg3})

	fmt.Println("  Written JSONL:")
	fmt.Printf("  %s", buf.String())

	// Read messages back (simulating reading from a file)
	reader := bytes.NewReader(buf.Bytes())
	receiver := broker.NewIOReceiver(reader, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Filter only app logs using wildcard pattern
	msgs, err := receiver.Receive(ctx, "logs/app")
	if err != nil {
		fmt.Printf("Error receiving: %v\n", err)
		return
	}

	fmt.Println("  Filtered messages (logs/app only):")
	for _, msg := range msgs {
		level := msg.Properties["level"]
		fmt.Printf("    [%s] %s\n", level, msg.Payload)
	}
}

func ioBrokerPipeExample() {
	// Use pipes for inter-process communication (IPC)
	// Messages are streamed as JSON between reader and writer

	// Note: IOBroker now uses []byte payload, not custom types
	pr, pw := io.Pipe()
	b := broker.NewIOBroker(pr, pw, broker.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start receiver in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond) // Wait for sends
		msgs, err := b.Receive(ctx, "events")
		if err != nil {
			fmt.Printf("Error receiving: %v\n", err)
			return
		}
		for _, msg := range msgs {
			fmt.Printf("  Received via pipe: %s\n", msg.Payload)
		}
	}()

	// Send events through the pipe
	go func() {
		b.Send(ctx, "events", []*message.Message{message.New([]byte("user.created: user-123"), message.Properties{})})
		b.Send(ctx, "events", []*message.Message{message.New([]byte("order.placed: order-456"), message.Properties{})})
		pw.Close() // Signal EOF
	}()

	wg.Wait()
}

func httpBrokerExample() {
	// HTTP broker uses POST requests with:
	// - X-Gopipe-Topic header for the topic
	// - X-Gopipe-Prop-* headers for message properties
	// - Request body contains the payload
	// Returns 201 Created on success

	// Note: HTTP broker functions are currently disabled in broker package
	// This example would need to be updated once http.go is re-enabled
	fmt.Println("  HTTP broker example is currently disabled (http.go needs refactoring)")

	// Placeholder for future implementation:
	// receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	// defer receiver.Close()
	// 
	// server := httptest.NewServer(receiver.Handler())
	// defer server.Close()
	// 
	// sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
	// 	Headers: map[string]string{"Authorization": "Bearer webhook-token"},
	// })
	//
	// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	// defer cancel()
	//
	// msgs, _ := receiver.Receive(ctx, "webhooks/github")
	// for _, msg := range msgs {
	// 	fmt.Printf("  Received webhook: %s\n", msg.Payload)
	// }
}
