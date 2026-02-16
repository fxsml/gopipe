// Example: JSON Schema validated messaging.
//
// Demonstrates a custom Marshaler that validates CloudEvents payloads
// against explicit JSON Schema definitions. This solves the Go zero-value
// dilemma: "Is amount really 0, or was the field missing?"
//
// CloudEvents defines the message envelope contract (type, source, id).
// JSON Schema defines the payload data contract (what's inside "data").
// Together they provide a full event-driven API spec.
//
// Run: go run ./examples/07-validating-marshaler
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/jsonschema"
	"github.com/google/uuid"
)

// Schema is the source of truth — the data contract.
// Go structs conform to the schema, not the other way around.
const createOrderSchema = `{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"type": "object",
	"properties": {
		"order_id": { "type": "string", "minLength": 1 },
		"amount":   { "type": "number", "exclusiveMinimum": 0 }
	},
	"required": ["order_id", "amount"],
	"additionalProperties": false
}`

// CreateOrder is the input command. Fields match the schema above.
type CreateOrder struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// OrderCreated is the output event.
type OrderCreated struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	// Create marshaler and register schema for the input type.
	marshaler := jsonschema.NewMarshaler()
	marshaler.MustRegister(CreateOrder{}, createOrderSchema)

	engine := message.NewEngine(message.EngineConfig{
		Marshaler: marshaler,
		ErrorHandler: func(msg *message.Message, err error) {
			fmt.Printf("Rejected: %v\n", err)
		},
	})

	handler := message.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			fmt.Printf("Accepted: order_id=%s amount=%.2f\n", cmd.OrderID, cmd.Amount)
			return []OrderCreated{{OrderID: cmd.OrderID, Status: "created"}}, nil
		},
		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
	)
	engine.AddHandler("process-order", nil, handler)

	input := make(chan *message.RawMessage, 10)
	engine.AddRawInput("input", nil, input)
	output, _ := engine.AddRawOutput("output", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	send := func(data []byte) {
		input <- message.NewRaw(data, message.Attributes{
			message.AttrSpecVersion: "1.0",
			message.AttrType:        "create.order",
			message.AttrSource:      "/test",
			message.AttrID:          uuid.NewString(),
		}, nil)
	}

	// 1. Valid — all required fields present with correct types.
	valid, _ := json.Marshal(CreateOrder{OrderID: "ORD-1", Amount: 49.99})
	send(valid)
	fmt.Printf("Output: %s\n", <-output)

	// 2. Missing "amount" — Go would zero-fill to 0.0, but schema catches it.
	send([]byte(`{"order_id":"ORD-2"}`))

	// 3. Wrong type — "amount" is a string, not a number.
	send([]byte(`{"order_id":"ORD-3","amount":"free"}`))

	// 4. Zero-length order_id — Go sees "", schema enforces minLength: 1.
	send([]byte(`{"order_id":"","amount":10}`))

	// Let engine drain invalid messages before shutting down.
	time.Sleep(100 * time.Millisecond)
	close(input)
	cancel()
	<-done
}
