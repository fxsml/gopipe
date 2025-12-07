package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

type Order struct {
	ID     string
	Amount int
}

func main() {
	ctx := context.Background()

	// Simulate message broker integration with ack/nack callbacks
	ack := func() { fmt.Println("✓ Message acknowledged") }
	nack := func(err error) { fmt.Printf("✗ Message rejected: %v\n", err) }

	// Create messages using non-generic Message type (pub/sub pattern)
	deadline := time.Now().Add(30 * time.Second)

	// Marshal orders to []byte for messaging
	order1, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	order2, _ := json.Marshal(Order{ID: "order-2", Amount: 200})

	in := channel.FromValues(
		message.New(order1, message.Properties{
			message.PropDeadline:   deadline,
			message.PropID:         "msg-001",
			message.PropSubject:    "orders.created",
			message.PropCreatedAt:  time.Now(),
		}, message.NewAcking(ack, nack)),
		message.New(order2, message.Properties{
			message.PropDeadline:   deadline,
			message.PropID:         "msg-002",
			message.PropSubject:    "orders.created",
			message.PropCreatedAt:  time.Now(),
		}, message.NewAcking(ack, nack)),
	)

	// Create pipe with acknowledgment - using non-generic Message
	pipe := gopipe.NewTransformPipe(
		func(ctx context.Context, msg *message.Message) (*message.Message, error) {
			msg.Properties["processed_at"] = time.Now().Format(time.RFC3339)

			// Unmarshal the order
			var order Order
			if err := json.Unmarshal(msg.Payload, &order); err != nil {
				err := fmt.Errorf("failed to unmarshal order: %w", err)
				msg.Nack(err)
				return nil, err
			}

			// Simulate processing error for order-1
			if order.ID == "order-1" {
				err := fmt.Errorf("cannot process order %s", order.ID)
				msg.Nack(err)
				return nil, err
			}

			// Process successfully - double the amount
			order.Amount *= 2

			// Marshal back to []byte
			processedData, _ := json.Marshal(order)

			// On success
			msg.Ack()
			result := message.Copy(msg, processedData)
			result.Properties[message.PropSubject] = "orders.processed"
			return result, nil
		},
	)

	// Process messages
	results := pipe.Start(ctx, in)

	// Consume results
	<-channel.Sink(results, func(result *message.Message) {
		var order Order
		json.Unmarshal(result.Payload, &order)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Order ID: %s, Amount: %d\n", order.ID, order.Amount))
		sb.WriteString("Properties:\n")
		for key, value := range result.Properties {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
		fmt.Print(sb.String())
	})
}
