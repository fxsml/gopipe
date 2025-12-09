package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
)

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("Multiplex Publisher/Subscriber Example")
	fmt.Println("Route messages to different brokers based on topic patterns")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()

	// Run examples
	example1_PrefixRouting()
	example2_PatternRouting()
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
	fmt.Println("Route internal.* topics to memory broker, everything else to external")
	fmt.Println()

	// Create brokers
	memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	externalBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

	// Create multiplex sender with routing logic
	selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
	multiplexSender := pubsub.NewMultiplexSender(selector, externalBroker)

	// Create publisher using multiplex sender
	publisher := pubsub.NewPublisher(
		multiplexSender,
		pubsub.RouteBySubject(),
		pubsub.PublisherConfig{
			MaxBatchSize: 10,
		},
	)

	ctx := context.Background()
	msgs := make(chan *message.Message, 10)

	// Start publisher
	done := publisher.Publish(ctx, msgs)

	// Send messages with different subjects
	msgs <- message.New([]byte("Fast cache update"), message.Properties{
		message.PropSubject: "internal.cache.update",
	})
	msgs <- message.New([]byte("Internal event"), message.Properties{
		message.PropSubject: "internal.events.user.created",
	})
	msgs <- message.New([]byte("External API call"), message.Properties{
		message.PropSubject: "external.api.request",
	})
	msgs <- message.New([]byte("Order created"), message.Properties{
		message.PropSubject: "orders.created",
	})

	close(msgs)
	<-done

	// Verify routing
	fmt.Println("Routing results:")

	internalMsgs, _ := memoryBroker.Receive(ctx, "internal.cache.update")
	fmt.Printf("  ✓ Memory broker received %d message(s) for 'internal.cache.update'\n", len(internalMsgs))

	internalMsgs2, _ := memoryBroker.Receive(ctx, "internal.events.user.created")
	fmt.Printf("  ✓ Memory broker received %d message(s) for 'internal.events.user.created'\n", len(internalMsgs2))

	externalMsgs, _ := externalBroker.Receive(ctx, "external.api.request")
	fmt.Printf("  ✓ External broker received %d message(s) for 'external.api.request'\n", len(externalMsgs))

	orderMsgs, _ := externalBroker.Receive(ctx, "orders.created")
	fmt.Printf("  ✓ External broker received %d message(s) for 'orders.created'\n", len(orderMsgs))

	fmt.Println()
}

// ============================================================================
// Example 2: Topic Pattern Routing
// ============================================================================

func example2_PatternRouting() {
	fmt.Println("--- Example 2: Topic Pattern Routing ---")
	fmt.Println("Use wildcard patterns to route messages")
	fmt.Println()

	// Create brokers for different purposes
	memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	auditBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	natsBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{}) // Simulated NATS

	// Define routing rules (first match wins)
	selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
		{Pattern: "internal.*", Sender: memoryBroker},      // internal.* → memory
		{Pattern: "audit.**", Sender: auditBroker},         // audit.** → audit system
		{Pattern: "events.us.*", Sender: natsBroker},       // US events → NATS
		{Pattern: "events.eu.*", Sender: natsBroker},       // EU events → NATS
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
		{"internal.cache", "Cache update", "Memory"},
		{"audit.security.login", "User login audit", "Audit"},
		{"audit.us.transaction", "Transaction audit", "Audit"},
		{"events.us.order.created", "US order", "NATS"},
		{"events.eu.user.registered", "EU user", "NATS"},
		{"random.topic", "Random message", "NATS (fallback)"},
	}

	fmt.Println("Routing patterns:")
	fmt.Println("  internal.*     → Memory broker")
	fmt.Println("  audit.**       → Audit broker")
	fmt.Println("  events.us.*    → NATS broker")
	fmt.Println("  events.eu.*    → NATS broker")
	fmt.Println("  *              → NATS broker (default)")
	fmt.Println()

	fmt.Println("Sending messages:")
	for _, tm := range testMessages {
		msg := message.New([]byte(tm.payload), message.Properties{})
		err := multiplexSender.Send(ctx, tm.topic, []*message.Message{msg})
		if err != nil {
			log.Printf("Error sending to %s: %v", tm.topic, err)
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
	fmt.Println("Combine multiple selection strategies (first match wins)")
	fmt.Println()

	memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	auditBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	priorityBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	natsBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

	// Chain multiple selectors
	selector := pubsub.ChainSenderSelectors(
		// First: check audit topics
		pubsub.PrefixSenderSelector("audit", auditBroker),

		// Second: check internal topics
		pubsub.PrefixSenderSelector("internal", memoryBroker),

		// Third: check for priority flag
		func(topic string) pubsub.Sender {
			if strings.Contains(topic, ".priority.") {
				return priorityBroker
			}
			return nil
		},
	)

	multiplexSender := pubsub.NewMultiplexSender(selector, natsBroker)

	ctx := context.Background()

	// Send test messages
	testMessages := []struct {
		topic   string
		payload string
		expectedBroker string
	}{
		{"audit.login", "Audit message", "Audit (1st selector)"},
		{"internal.cache", "Internal message", "Memory (2nd selector)"},
		{"orders.priority.urgent", "Priority message", "Priority (3rd selector)"},
		{"regular.message", "Regular message", "NATS (fallback)"},
	}

	fmt.Println("Selector chain:")
	fmt.Println("  1. audit*       → Audit broker")
	fmt.Println("  2. internal*    → Memory broker")
	fmt.Println("  3. *.priority.* → Priority broker")
	fmt.Println("  4. (fallback)   → NATS broker")
	fmt.Println()

	fmt.Println("Sending messages:")
	for _, tm := range testMessages {
		msg := message.New([]byte(tm.payload), message.Properties{})
		err := multiplexSender.Send(ctx, tm.topic, []*message.Message{msg})
		if err != nil {
			log.Printf("Error: %v", err)
		}
		fmt.Printf("  ✓ %s → %s\n", tm.topic, tm.expectedBroker)
	}
	fmt.Println()
}

// ============================================================================
// Example 4: Multiplex Receiver (Subscriptions)
// ============================================================================

func example4_MultiplexReceiver() {
	fmt.Println("--- Example 4: Multiplex Receiver (Subscriptions) ---")
	fmt.Println("Route subscriptions to different brokers")
	fmt.Println()

	// Create brokers with pre-populated messages
	memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
	natsBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

	ctx := context.Background()

	// Pre-populate brokers
	memoryBroker.Send(ctx, "internal.events", []*message.Message{
		message.New([]byte("Internal event 1"), message.Properties{}),
		message.New([]byte("Internal event 2"), message.Properties{}),
	})

	natsBroker.Send(ctx, "external.api", []*message.Message{
		message.New([]byte("External API event 1"), message.Properties{}),
	})

	// Create multiplex receiver
	selector := pubsub.PrefixReceiverSelector("internal", memoryBroker)
	multiplexReceiver := pubsub.NewMultiplexReceiver(selector, natsBroker)

	fmt.Println("Routing rules:")
	fmt.Println("  internal.* → Memory broker")
	fmt.Println("  *          → NATS broker (default)")
	fmt.Println()

	// Subscribe to internal topic (uses memory broker)
	fmt.Println("Receiving from 'internal.events' (should use Memory broker):")
	internalMsgs, err := multiplexReceiver.Receive(ctx, "internal.events")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	for i, msg := range internalMsgs {
		fmt.Printf("  ✓ Message %d: %s\n", i+1, string(msg.Payload))
	}
	fmt.Println()

	// Subscribe to external topic (uses NATS broker)
	fmt.Println("Receiving from 'external.api' (should use NATS broker):")
	externalMsgs, err := multiplexReceiver.Receive(ctx, "external.api")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	for i, msg := range externalMsgs {
		fmt.Printf("  ✓ Message %d: %s\n", i+1, string(msg.Payload))
	}
	fmt.Println()
}

// ============================================================================
// Bonus: Environment-Based Routing
// ============================================================================

func createEnvironmentRouter() pubsub.Sender {
	memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

	// In development: everything goes to memory broker for testing
	if false { // Change to: os.Getenv("ENV") == "development"
		return memoryBroker
	}

	// In production: use sophisticated routing
	natsBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

	selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
		{Pattern: "internal.*", Sender: memoryBroker},
		{Pattern: "test.*", Sender: memoryBroker},
	})

	return pubsub.NewMultiplexSender(selector, natsBroker)
}
