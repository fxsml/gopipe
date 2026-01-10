// Example: Message engine with CloudEvents handler.
//
// Demonstrates the core message engine concepts:
// - Creating an engine with a marshaler
// - Registering a command handler
// - Sending and receiving messages
//
// Run: go run ./examples/04-message
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/fxsml/gopipe/message"
)

// CreateOrder is the input command type.
type CreateOrder struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// OrderCreated is the output event type.
type OrderCreated struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	// Create engine with JSON marshaler
	engine := message.NewEngine(message.EngineConfig{
		Marshaler: message.NewJSONMarshaler(),
	})

	// Register a command handler
	// Input type (CreateOrder) determines which messages it handles
	// Output type (OrderCreated) determines the response event type
	handler := message.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			fmt.Printf("Processing order: %s (amount: %.2f)\n", cmd.OrderID, cmd.Amount)
			return []OrderCreated{{
				OrderID: cmd.OrderID,
				Status:  "created",
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/orders",
			Naming: message.KebabNaming, // CreateOrder -> "create.order"
		},
	)
	engine.AddHandler("process-order", nil, handler)

	// Create input channel and connect to engine
	input := make(chan *message.RawMessage, 10)
	engine.AddRawInput("input", nil, input)

	// Create output channel
	output, _ := engine.AddRawOutput("output", nil)

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Send a message
	data, _ := json.Marshal(CreateOrder{OrderID: "ORD-123", Amount: 99.99})
	input <- message.NewRaw(data, message.Attributes{
		"type":   "create.order",
		"source": "/test",
		"id":     "msg-1",
	}, nil)

	// Receive the response
	result := <-output
	fmt.Printf("Received: %s\n", result)

	// Shutdown
	close(input)
	cancel()
	<-done

	// Output:
	// Processing order: ORD-123 (amount: 99.99)
	// Received: {"data":{"order_id":"ORD-123","status":"created"},...}
}
