package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
)

func main() {
	fmt.Println("=== In-Memory Broker Example ===")
	fmt.Println()

	// Create channel broker with configuration
	b := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{
		CloseTimeout: 5 * time.Second,
		SendTimeout:  time.Second,
		BufferSize:   50,
	})
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Example 1: Basic pub/sub with Subscribe (push-based)
	fmt.Println("--- Example 1: Basic Pub/Sub (Subscribe) ---")
	basicPubSubSubscribe(ctx, b, &wg)
	wg.Wait()

	// Example 2: Multiple subscribers on same topic
	fmt.Println("\n--- Example 2: Multiple Subscribers ---")
	multipleSubscribers(ctx, b, &wg)
	wg.Wait()

	// Example 3: Hierarchical topics with exact match
	fmt.Println("\n--- Example 3: Hierarchical Topics ---")
	hierarchicalTopics(ctx, b, &wg)
	wg.Wait()

	// Example 4: Receive (polling mode) for compatibility
	fmt.Println("\n--- Example 4: Receive (Polling Mode) ---")
	receivePollingMode(ctx, b, &wg)
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

func basicPubSubSubscribe(ctx context.Context, b *pubsub.ChannelBroker, wg *sync.WaitGroup) {
	// Subscribe FIRST - returns a channel for real-time message delivery
	ch := b.Subscribe(ctx, "greetings")

	// Collect messages in background
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Printf("  Received: %s\n", msg.Payload)
			case <-time.After(100 * time.Millisecond):
				return
			}
		}
	}()

	// Give subscriber time to start
	time.Sleep(10 * time.Millisecond)

	// Publish messages - they are delivered to active subscribers
	b.Send(ctx, "greetings", []*message.Message{
		message.New([]byte("Hello, World!"), message.Properties{}),
	})
	b.Send(ctx, "greetings", []*message.Message{
		message.New([]byte("Welcome to gopipe!"), message.Properties{}),
	})
}

func multipleSubscribers(ctx context.Context, b *pubsub.ChannelBroker, wg *sync.WaitGroup) {
	// Create multiple subscribers for the same topic
	ch1 := b.Subscribe(ctx, "events")
	ch2 := b.Subscribe(ctx, "events")
	ch3 := b.Subscribe(ctx, "events")

	// Start goroutines to receive messages
	for i, ch := range []<-chan *message.Message{ch1, ch2, ch3} {
		wg.Add(1)
		go func(id int, c <-chan *message.Message) {
			defer wg.Done()
			for {
				select {
				case msg, ok := <-c:
					if !ok {
						return
					}
					fmt.Printf("  Subscriber %d received: %s\n", id, msg.Payload)
				case <-time.After(100 * time.Millisecond):
					return
				}
			}
		}(i+1, ch)
	}

	time.Sleep(10 * time.Millisecond)

	// Send event - ALL subscribers receive it (fan-out)
	b.Send(ctx, "events", []*message.Message{
		message.New([]byte("Important event occurred!"), message.Properties{}),
	})
}

func hierarchicalTopics(ctx context.Context, b *pubsub.ChannelBroker, wg *sync.WaitGroup) {
	// Subscribe to specific topics (exact match only - no wildcards)
	chOrders := b.Subscribe(ctx, "orders/created")
	chUpdates := b.Subscribe(ctx, "orders/updated")
	chProfile := b.Subscribe(ctx, "users/profile/updated")

	// Collect messages from all channels
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg, ok := <-chOrders:
				if !ok {
					return
				}
				fmt.Printf("  [orders/created] %s\n", msg.Payload)
			case msg, ok := <-chUpdates:
				if !ok {
					return
				}
				fmt.Printf("  [orders/updated] %s\n", msg.Payload)
			case msg, ok := <-chProfile:
				if !ok {
					return
				}
				fmt.Printf("  [users/profile/updated] %s\n", msg.Payload)
			case <-time.After(100 * time.Millisecond):
				return
			}
		}
	}()

	time.Sleep(10 * time.Millisecond)

	// Send to different topics
	b.Send(ctx, "orders/created", []*message.Message{
		message.New([]byte("Order #123 created"), message.Properties{}),
	})
	b.Send(ctx, "orders/updated", []*message.Message{
		message.New([]byte("Order #456 shipped"), message.Properties{}),
	})
	b.Send(ctx, "users/profile/updated", []*message.Message{
		message.New([]byte("User john updated profile"), message.Properties{}),
	})
}

func receivePollingMode(ctx context.Context, b *pubsub.ChannelBroker, wg *sync.WaitGroup) {
	// Receive() creates a temporary subscription and polls for ~100ms
	// Use this for one-shot polling or compatibility with existing code

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Start sending while Receive is waiting
		time.Sleep(10 * time.Millisecond)
		b.Send(ctx, "polling/topic", []*message.Message{
			message.New([]byte("Polled message 1"), message.Properties{}),
			message.New([]byte("Polled message 2"), message.Properties{}),
		})
	}()

	// Receive waits for messages (blocking with 100ms timeout)
	msgs, err := b.Receive(ctx, "polling/topic")
	if err != nil {
		fmt.Printf("  Error receiving: %v\n", err)
		return
	}

	fmt.Printf("  Received %d message(s):\n", len(msgs))
	for _, msg := range msgs {
		fmt.Printf("    - %s\n", msg.Payload)
	}
}

func ioBrokerExample() {
	// IO broker writes messages as JSON Lines (JSONL)
	// This can be used for file-based persistence or log streaming

	// Write messages to a buffer (could be a file)
	var buf bytes.Buffer
	sender := pubsub.NewIOSender(&buf, pubsub.IOConfig{})

	ctx := context.Background()

	// Send messages with properties (payload must be valid JSON)
	msg1 := message.New([]byte(`"Application started"`), message.Properties{"id": "log-001", "level": "info"})
	sender.Send(ctx, "logs/app", []*message.Message{msg1})

	msg2 := message.New([]byte(`"Processing request"`), message.Properties{"id": "log-002", "level": "debug"})
	sender.Send(ctx, "logs/app", []*message.Message{msg2})

	msg3 := message.New([]byte(`"Connection failed"`), message.Properties{"id": "log-003", "level": "error"})
	sender.Send(ctx, "logs/error", []*message.Message{msg3})

	fmt.Println("  Written JSONL:")
	fmt.Printf("  %s", buf.String())

	// Read messages back (simulating reading from a file)
	reader := bytes.NewReader(buf.Bytes())
	receiver := pubsub.NewIOReceiver(reader, pubsub.IOConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Filter only app logs using exact topic match
	msgs, err := receiver.Receive(ctx, "logs/app")
	if err != nil {
		fmt.Printf("  Error receiving: %v\n", err)
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

	pr, pw := io.Pipe()
	b := pubsub.NewIOBroker(pr, pw, pubsub.IOConfig{})

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
			fmt.Printf("  Error receiving: %v\n", err)
			return
		}
		for _, msg := range msgs {
			fmt.Printf("  Received via pipe: %s\n", msg.Payload)
		}
	}()

	// Send events through the pipe (payload must be valid JSON)
	go func() {
		b.Send(ctx, "events", []*message.Message{
			message.New([]byte(`"user.created: user-123"`), message.Properties{}),
		})
		b.Send(ctx, "events", []*message.Message{
			message.New([]byte(`"order.placed: order-456"`), message.Properties{}),
		})
		pw.Close() // Signal EOF
	}()

	wg.Wait()
}

func httpBrokerExample() {
	// HTTP broker uses POST requests with:
	// - X-Gopipe-Topic header for the topic
	// - X-Gopipe-Prop-* headers for message properties
	// - Request body contains the payload ([]byte)
	// Returns 201 Created on success (or 200 if WaitForAck is true)

	receiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)
	defer receiver.Close()

	// The receiver implements http.Handler, so you can use it directly
	fmt.Println("  HTTP Receiver created and ready to accept webhook POSTs")
	fmt.Println("  Example curl command:")
	fmt.Println(`    curl -X POST http://localhost:8080/webhook \`)
	fmt.Println(`      -H "X-Gopipe-Topic: webhooks/github" \`)
	fmt.Println(`      -H "X-Gopipe-Prop-event: push" \`)
	fmt.Println(`      -H "X-Gopipe-Prop-repo: gopipe" \`)
	fmt.Println(`      -d '{"commits": 3, "branch": "main"}'`)
	fmt.Println()

	// Use Handler() to get http.Handler (for http.ListenAndServe)
	_ = receiver.Handler()

	fmt.Println("  HTTP broker is fully functional with []byte payloads")
}
