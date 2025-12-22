package main

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
)

func main() {
	ctx := context.Background()

	// Simulate message broker integration with ack/nack callbacks
	ack := func() { fmt.Println("✓ Message acknowledged") }
	nack := func(err error) { fmt.Printf("✗ Message rejected: %v\n", err) }

	// Create messages using the new direct construction API
	in := channel.FromValues(
		message.NewWithAcking(12, message.Attributes{
			message.AttrID:   "msg-001",
			"source":         "orders-queue",
			message.AttrTime: time.Now(),
		}, ack, nack),
		message.NewWithAcking(42, message.Attributes{
			message.AttrID:   "msg-002",
			"source":         "orders-queue",
			message.AttrTime: time.Now(),
		}, ack, nack),
	)

	// Create pipe with acknowledgment
	pipe := pipe.NewTransformPipe(
		func(ctx context.Context, msg *message.TypedMessage[int]) (*message.TypedMessage[int], error) {
			// Add processed timestamp
			msg.Attributes["processed_at"] = time.Now().Format(time.RFC3339)

			// Simulate processing error
			if msg.Data == 12 {
				err := fmt.Errorf("cannot process payload 12")
				msg.Nack(err)
				return nil, err
			}

			// On success
			res := msg.Data * 2
			msg.Ack()
			return message.New(res, msg.Attributes), nil
		}, pipe.Config{},
	)

	// Process message
	results, err := pipe.Start(ctx, in)
	if err != nil {
		panic(err)
	}

	// Consume results
	<-channel.Sink(results, func(result *message.TypedMessage[int]) {
		fmt.Printf("Payload: %d\nAttributes:\n", result.Data)
		for k, v := range result.Attributes {
			fmt.Printf("  %s: %v\n", k, v)
		}
	})
}
