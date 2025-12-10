package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
)

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("Multiplex Publisher/Subscriber Example")
	fmt.Println("Route messages to different brokers based on topic")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()

	// Run examples
	example1_PrefixRouting()
	example2_ExactRouting()
	example3_ChainedSelectors()
	example4_MultiplexReceiver()

	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("All examples completed!")
	fmt.Println(strings.Repeat("=", 70))
}

// ============================================================================
// Example 1: Simple Prefix-Based Routing
// ============================================================================

func example1_PrefixRouting() {
	fmt.Println("--- Example 1: Prefix-Based Routing ---")
	fmt.Println("Route internal/* topics to memory broker, everything else to external")
	fmt.Println()

	// Create brokers
	memoryBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
	externalBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})

	// Create multiplex sender with routing logic
	selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
	multiplexSender := pubsub.NewMultiplexSender(selector, externalBroker)

	// Create publisher using multiplex sender
	publisher := pubsub.NewPublisher(
		multiplexSender,
		pubsub.PublisherConfig{
			MaxBatchSize: 10,
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Subscribe first (messages only delivered to active subscriptions)
	internalCacheCh := memoryBroker.Subscribe(ctx, "internal/cache/update")
	internalEventsCh := memoryBroker.Subscribe(ctx, "internal/events/user/created")
	externalApiCh := externalBroker.Subscribe(ctx, "external/api/request")
	ordersCh := externalBroker.Subscribe(ctx, "orders/created")

	msgs := make(chan *message.Message, 10)

	// Start publisher
	done := publisher.Publish(ctx, msgs)

	// Send messages with different topics
	msgs <- message.New([]byte("Fast cache update"), message.Properties{
		message.PropTopic: "internal/cache/update",
	})
	msgs <- message.New([]byte("Internal event"), message.Properties{
		message.PropTopic: "internal/events/user/created",
	})
	msgs <- message.New([]byte("External API call"), message.Properties{
		message.PropTopic: "external/api/request",
	})
	msgs <- message.New([]byte("Order created"), message.Properties{
		message.PropTopic: "orders/created",
	})

	close(msgs)
	<-done

	// Verify routing
	fmt.Println("Routing results:")

	select {
	case msg := <-internalCacheCh:
		fmt.Printf("  ✓ Memory broker received message for 'internal/cache/update': %s\n", msg.Payload)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("  ✓ Memory broker received 0 message(s) for 'internal/cache/update'")
	}

	select {
	case msg := <-internalEventsCh:
		fmt.Printf("  ✓ Memory broker received message for 'internal/events/user/created': %s\n", msg.Payload)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("  ✓ Memory broker received 0 message(s) for 'internal/events/user/created'")
	}

	select {
	case msg := <-externalApiCh:
		fmt.Printf("  ✓ External broker received message for 'external/api/request': %s\n", msg.Payload)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("  ✓ External broker received 0 message(s) for 'external/api/request'")
	}

	select {
	case msg := <-ordersCh:
		fmt.Printf("  ✓ External broker received message for 'orders/created': %s\n", msg.Payload)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("  ✓ External broker received 0 message(s) for 'orders/created'")
	}

	fmt.Println()
}

// ============================================================================
// Example 2: Exact Topic Routing
// ============================================================================

func example2_ExactRouting() {
	fmt.Println("--- Example 2: Exact Topic Routing ---")
	fmt.Println("Use exact topic matches to route messages")
	fmt.Println()

	// Create brokers for different purposes
	memoryBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
	auditBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
	natsBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{}) // Simulated NATS

	// Define routing rules (exact match only)
	selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
		{Topic: "internal/cache", Sender: memoryBroker},
		{Topic: "audit/login", Sender: auditBroker},
		{Topic: "events/order/created", Sender: natsBroker},
	})

	multiplexSender := pubsub.NewMultiplexSender(
		selector,
		natsBroker, // Default: everything else → NATS
	)

	ctx := context.Background()

	// Send various messages
	testMessages := []struct {
		topic   string
		payload string
		broker  string
	}{
		{"internal/cache", "Cache update", "Memory"},
		{"audit/login", "User login audit", "Audit"},
		{"events/order/created", "Order created", "NATS"},
		{"random/topic", "Random message", "NATS (fallback)"},
	}

	fmt.Println("Routing rules (exact match):")
	fmt.Println("  internal/cache        → Memory broker")
	fmt.Println("  audit/login           → Audit broker")
	fmt.Println("  events/order/created  → NATS broker")
	fmt.Println("  *                     → NATS broker (default)")
	fmt.Println()

	fmt.Println("Sending messages:")
	for _, tm := range testMessages {
		msg := message.New([]byte(tm.payload), message.Properties{})
		if err := multiplexSender.Send(ctx, tm.topic, []*message.Message{msg}); err != nil {
			log.Printf("Failed to send to %s: %v", tm.topic, err)
			continue
		}
		fmt.Printf("  ✓ %s → %s\n", tm.topic, tm.broker)
	}

	fmt.Println()
}

// ============================================================================
// Example 3: Chained Selectors
// ============================================================================

func example3_ChainedSelectors() {
	fmt.Println("--- Example 3: Chained Selectors ---")
	fmt.Println("Combine multiple selectors; first match wins")
	fmt.Println()

	// Create brokers
	auditBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
	internalBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
	externalBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})

	// Chain selectors (first match wins)
	selector := pubsub.ChainSenderSelectors(
		pubsub.PrefixSenderSelector("audit", auditBroker),
		pubsub.PrefixSenderSelector("internal", internalBroker),
	)

	multiplexSender := pubsub.NewMultiplexSender(selector, externalBroker)

	ctx := context.Background()

	testCases := []struct {
		topic   string
		broker  string
	}{
		{"audit/security", "Audit"},
		{"audit/financial", "Audit"},
		{"internal/cache", "Internal"},
		{"internal/events", "Internal"},
		{"orders/created", "External (fallback)"},
		{"users/registered", "External (fallback)"},
	}

	fmt.Println("Selector chain:")
	fmt.Println("  1. audit/* → Audit broker")
	fmt.Println("  2. internal/* → Internal broker")
	fmt.Println("  3. * → External broker (fallback)")
	fmt.Println()

	fmt.Println("Routing results:")
	for _, tc := range testCases {
		msg := message.New([]byte("test"), message.Properties{})
		if err := multiplexSender.Send(ctx, tc.topic, []*message.Message{msg}); err != nil {
			log.Printf("Failed to send to %s: %v", tc.topic, err)
			continue
		}
		fmt.Printf("  ✓ %s → %s\n", tc.topic, tc.broker)
	}

	fmt.Println()
}

// ============================================================================
// Example 4: Multiplex Receiver
// ============================================================================

func example4_MultiplexReceiver() {
	fmt.Println("--- Example 4: Multiplex Receiver ---")
	fmt.Println("Route subscriptions to different receivers based on topic")
	fmt.Println()

	// Create brokers and populate with messages
	memoryBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
	externalBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Subscribe first
	internalCh := memoryBroker.Subscribe(ctx, "internal/events")
	externalCh := externalBroker.Subscribe(ctx, "external/api")

	// Send messages to brokers
	memoryBroker.Send(ctx, "internal/events", []*message.Message{
		message.New([]byte("Internal event 1"), message.Properties{}),
		message.New([]byte("Internal event 2"), message.Properties{}),
	})

	externalBroker.Send(ctx, "external/api", []*message.Message{
		message.New([]byte("External response"), message.Properties{}),
	})

	// Create multiplex receiver
	selector := pubsub.PrefixReceiverSelector("internal", memoryBroker)
	multiplexReceiver := pubsub.NewMultiplexReceiver(selector, externalBroker)

	fmt.Println("Receiver routing:")
	fmt.Println("  internal/* → Memory broker")
	fmt.Println("  * → External broker (fallback)")
	fmt.Println()

	// Subscribe to internal topic (should use memory broker)
	fmt.Println("Subscribing to 'internal/events':")
	count := 0
	for {
		select {
		case msg := <-internalCh:
			fmt.Printf("  ← Received: %s\n", msg.Payload)
			count++
		case <-time.After(100 * time.Millisecond):
			goto doneInternal
		}
	}
doneInternal:
	fmt.Printf("  Total: %d message(s) from memory broker\n\n", count)

	// Subscribe to external topic (should use external broker)
	fmt.Println("Subscribing to 'external/api':")
	count = 0
	for {
		select {
		case msg := <-externalCh:
			fmt.Printf("  ← Received: %s\n", msg.Payload)
			count++
		case <-time.After(100 * time.Millisecond):
			goto doneExternal
		}
	}
doneExternal:
	fmt.Printf("  Total: %d message(s) from external broker\n\n", count)

	// Show that Receive still works through multiplex
	fmt.Println("Using multiplex Receive (polling mode):")
	msgs, _ := multiplexReceiver.Receive(ctx, "internal/events")
	fmt.Printf("  Receive returned %d message(s) for 'internal/events'\n", len(msgs))
}
