package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/fxsml/gopipe/message"
)

// simulateMessageBroker demonstrates how a message broker would use ack/nack
// to implement at-least-once delivery semantics.
func simulateMessageBroker() {
	fmt.Println("=== Message Broker with Ack/Nack ===")
	fmt.Println()

	// Simulate message queue
	messages := []struct {
		id      string
		payload int
	}{
		{"msg-1", 10},
		{"msg-2", 20},
		{"msg-3", -1}, // This will cause an error
		{"msg-4", 30},
	}

	// Track message status
	acked := make(map[string]bool)
	nacked := make(map[string]error)

	// Create input channel
	in := make(chan *message.Message[int], len(messages))

	// Enqueue messages with ack/nack handlers
	for _, msg := range messages {
		id := msg.id
		in <- message.NewMessageWithAck(
			id,
			msg.payload,
			func() {
				acked[id] = true
				fmt.Printf("✓ Message %s acknowledged\n", id)
			},
			func(err error) {
				nacked[id] = err
				fmt.Printf("✗ Message %s nacked: %v\n", id, err)
			},
		)
	}
	close(in)

	// Create processing pipe with business logic
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			// Simulate processing
			time.Sleep(10 * time.Millisecond)

			// Business logic: reject negative values
			if value < 0 {
				return nil, errors.New("negative value not allowed")
			}

			// Transform: double the value
			return []int{value * 2}, nil
		},
	)

	// Process messages
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out := pipe.Start(ctx, in)

	// Collect results
	fmt.Println("\nProcessing results:")
	for result := range out {
		fmt.Printf("  → Processed value: %d\n", result.Payload)
	}

	// Show final status
	fmt.Println("\nFinal Status:")
	fmt.Printf("  Acknowledged: %d messages\n", len(acked))
	fmt.Printf("  Nacked: %d messages\n", len(nacked))

	if len(nacked) > 0 {
		fmt.Println("\nMessages to retry:")
		for id, err := range nacked {
			fmt.Printf("  - %s (error: %v)\n", id, err)
		}
	}
}

// demonstrateDeadline shows how message deadlines work
func demonstrateDeadline() {
	fmt.Println()
	fmt.Println()
	fmt.Println("=== Message Deadline Example ===")
	fmt.Println()

	// Track status
	var status string

	// Create a message with a short deadline
	msg := message.NewMessageWithAck(
		"timeout-msg",
		42,
		func() {
			status = "acknowledged"
		},
		func(err error) {
			status = fmt.Sprintf("nacked: %v", err)
		},
	)
	msg.SetTimeout(50 * time.Millisecond)

	// Create input channel
	in := make(chan *message.Message[int], 1)
	in <- msg
	close(in)

	// Create pipe with slow processing
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value int) ([]int, error) {
			fmt.Println("Starting slow processing...")

			// Try to process but respect context
			select {
			case <-time.After(200 * time.Millisecond):
				fmt.Println("Processing complete (won't reach here)")
				return []int{value * 2}, nil
			case <-ctx.Done():
				fmt.Println("Processing interrupted by deadline")
				return nil, ctx.Err()
			}
		},
	)

	// Process message
	ctx := context.Background()
	out := pipe.Start(ctx, in)

	// Wait for completion
	for range out {
		// Drain output
	}

	// Give time for nack to be called
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("\nMessage status: %s\n", status)
}

// demonstrateNoAckNack shows that ack/nack is optional
func demonstrateNoAckNack() {
	fmt.Println()
	fmt.Println()
	fmt.Println("=== Optional Ack/Nack Example ===")
	fmt.Println()
	fmt.Println("Messages work fine without ack/nack handlers")
	fmt.Println()

	// Create message without ack/nack
	in := make(chan *message.Message[string], 2)
	in <- message.NewMessage("1", "hello")
	in <- message.NewMessage("2", "world")
	close(in)

	// Create pipe
	pipe := message.NewProcessPipe(
		func(ctx context.Context, value string) ([]string, error) {
			return []string{value + "!"}, nil
		},
	)

	// Process
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	out := pipe.Start(ctx, in)

	fmt.Println("Results:")
	for result := range out {
		fmt.Printf("  → %s\n", result.Payload)
	}
}

func main() {
	// Disable default logging for cleaner output
	log.SetOutput(io.Discard)

	// Run examples
	simulateMessageBroker()
	demonstrateDeadline()
	demonstrateNoAckNack()

	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Println(`The ack/nack pattern provides:

1. Automatic acknowledgment on success
2. Automatic negative acknowledgment on failure
3. Support for message deadlines
4. Optional - works without ack/nack handlers
5. Thread-safe - prevents double ack/nack
6. Integrates with retry and error handling

Use cases:
- Message queues (RabbitMQ, SQS, etc.)
- Event processing with delivery guarantees
- Reliable distributed systems
- At-least-once delivery semantics`)
}
