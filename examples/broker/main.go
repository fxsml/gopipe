package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func main() {
	fmt.Println("=== In-Memory Broker Example ===")
	fmt.Println()

	// Create broker with configuration
	b := broker.NewBroker[string](broker.Config{
		CloseTimeout: 5 * time.Second,
		SendTimeout:  time.Second,
		BufferSize:   50,
	})
	defer b.Close()

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
	fmt.Println("\n=== All examples completed ===")
}

func basicPubSub(ctx context.Context, b broker.Broker[string], wg *sync.WaitGroup) {
	// Get sender and receiver interfaces
	sender := broker.NewSender(b)
	receiver := broker.NewReceiver(b)

	// Start subscriber
	msgs := receiver.Receive(ctx, "greetings")

	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range msgs {
			fmt.Printf("  Received: %s\n", msg.Payload())
		}
	}()

	// Give subscriber time to connect
	time.Sleep(10 * time.Millisecond)

	// Publish messages
	sender.Send(ctx, "greetings", message.New("Hello, World!"))
	sender.Send(ctx, "greetings", message.New("Welcome to gopipe!"))

	time.Sleep(50 * time.Millisecond)
}

func multipleSubscribers(ctx context.Context, b broker.Broker[string], wg *sync.WaitGroup) {
	// Create multiple subscribers for the same topic
	sub1 := b.Receive(ctx, "events")
	sub2 := b.Receive(ctx, "events")

	// Process messages from both subscribers
	for i, sub := range []<-chan *message.Message[string]{sub1, sub2} {
		wg.Add(1)
		go func(id int, ch <-chan *message.Message[string]) {
			defer wg.Done()
			for msg := range ch {
				fmt.Printf("  Subscriber %d received: %s\n", id, msg.Payload())
			}
		}(i+1, sub)
	}

	time.Sleep(10 * time.Millisecond)

	// Send event - both subscribers will receive it
	b.Send(ctx, "events", message.New("Important event occurred!"))

	time.Sleep(50 * time.Millisecond)
}

func hierarchicalTopics(ctx context.Context, b broker.Broker[string], wg *sync.WaitGroup) {
	// Subscribe to different hierarchical topics
	ordersCreated := b.Receive(ctx, "orders/created")
	ordersUpdated := b.Receive(ctx, "orders/updated")
	usersProfile := b.Receive(ctx, "users/profile/updated")

	// Merge all channels using gopipe's channel.Merge
	merged := channel.Merge(ordersCreated, ordersUpdated, usersProfile)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range merged {
			fmt.Printf("  Received on merged channel: %s\n", msg.Payload())
		}
	}()

	time.Sleep(10 * time.Millisecond)

	// Send to different topics
	b.Send(ctx, "orders/created", message.New("Order #123 created"))
	b.Send(ctx, "orders/updated", message.New("Order #456 shipped"))
	b.Send(ctx, "users/profile/updated", message.New("User john updated profile"))

	time.Sleep(50 * time.Millisecond)
}

func gopipeIntegration(ctx context.Context, b broker.Broker[string], wg *sync.WaitGroup) {
	// Subscribe to incoming messages
	incoming := b.Receive(ctx, "input")

	// Create a processing pipeline that transforms messages
	pipe := gopipe.NewTransformPipe(
		func(ctx context.Context, msg *message.Message[string]) (*message.Message[string], error) {
			// Transform: uppercase the payload
			transformed := fmt.Sprintf("[PROCESSED] %s", msg.Payload())
			return message.Copy(msg, transformed), nil
		},
	)

	// Start pipeline
	processed := pipe.Start(ctx, incoming)

	// Consume processed messages and forward to output topic
	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range processed {
			fmt.Printf("  Pipeline output: %s\n", msg.Payload())
			// Forward to another topic
			b.Send(ctx, "output", msg)
		}
	}()

	// Subscribe to output topic
	output := b.Receive(ctx, "output")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range output {
			fmt.Printf("  Final output topic: %s\n", msg.Payload())
		}
	}()

	time.Sleep(10 * time.Millisecond)

	// Send messages through the pipeline
	b.Send(ctx, "input", message.New("message one"))
	b.Send(ctx, "input", message.New("message two"))

	time.Sleep(100 * time.Millisecond)
}
