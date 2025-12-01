package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

func main() {
	ctx := context.Background()

	// Simulate message broker integration with ack/nack callbacks
	ack := func() { fmt.Println("✓ Message acknowledged") }
	nack := func(err error) { fmt.Printf("✗ Message rejected: %v\n", err) }

	// Create messages using the new functional options API
	deadline := time.Now().Add(30 * time.Second)
	in := channel.FromValues(
		message.New(12,
			message.WithDeadline[int](deadline),
			message.WithAcking[int](ack, nack),
			message.WithID[int]("msg-001"),
			message.WithProperty[int]("source", "orders-queue"),
			message.WithCreatedAt[int](time.Now()),
		),
		message.New(42,
			message.WithDeadline[int](deadline),
			message.WithAcking[int](ack, nack),
			message.WithID[int]("msg-002"),
			message.WithProperty[int]("source", "orders-queue"),
			message.WithCreatedAt[int](time.Now()),
		),
	)

	// Create pipe with acknowledgment
	pipe := gopipe.NewTransformPipe(
		func(ctx context.Context, msg *message.Message[int]) (*message.Message[int], error) {
			defer msg.Properties().Set("processed_at", time.Now().Format(time.RFC3339))

			// Simulate processing error
			p := msg.Payload()
			if p == 12 {
				err := fmt.Errorf("cannot process payload 12")
				msg.Nack(err)
				return nil, err
			}

			// On success
			res := p * 2
			msg.Ack()
			return message.Copy(msg, res), nil
		},
	)

	// Process message
	results := pipe.Start(ctx, in)

	// Consume results
	<-channel.Sink(results, func(result *message.Message[int]) {
		var sb strings.Builder
		result.Properties().Range(func(key string, value any) bool {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
			return true
		})
		fmt.Printf("Payload: %d\nProperties:\n%s", result.Payload(), sb.String())
	})
}
