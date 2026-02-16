// Example: ValidatingMarshaler with JSON Schema validation.
//
// Demonstrates using ValidatingMarshaler to enforce a JSON Schema
// derived from Go struct tags via github.com/google/jsonschema-go.
// Invalid messages are rejected before they reach any handler.
//
// Run: go run ./examples/07-validating-marshaler
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/fxsml/gopipe/message"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
)

// CreateOrder is the input command type.
// JSON Schema is inferred from struct tags.
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
	// Generate a JSON Schema from the CreateOrder struct.
	schema, err := jsonschema.For[CreateOrder](nil)
	if err != nil {
		log.Fatalf("generating schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		log.Fatalf("resolving schema: %v", err)
	}

	// Build a DataValidator that checks bytes against the schema.
	validator := func(data []byte, _ any) error {
		var instance any
		if err := json.Unmarshal(data, &instance); err != nil {
			return fmt.Errorf("decoding JSON for validation: %w", err)
		}
		return resolved.Validate(instance)
	}

	// Wrap the JSON marshaler with pre-unmarshal validation.
	marshaler := message.NewValidatingMarshaler(
		message.NewJSONMarshaler(),
		message.ValidatingMarshalerConfig{
			UnmarshalValidation: validator,
		},
	)

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
		message.CommandHandlerConfig{
			Source: "/orders",
			Naming: message.KebabNaming,
		},
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

	// 1. Valid message — passes schema validation.
	valid, _ := json.Marshal(CreateOrder{OrderID: "ORD-1", Amount: 49.99})
	input <- message.NewRaw(valid, message.Attributes{
		message.AttrSpecVersion: "1.0",
		message.AttrType:        "create.order",
		message.AttrSource:      "/test",
		message.AttrID:          uuid.NewString(),
	}, nil)

	fmt.Printf("Output: %s\n", <-output)

	// 2. Invalid message — "amount" has wrong type; rejected by schema.
	invalid := []byte(`{"order_id":"ORD-2","amount":"not-a-number"}`)
	input <- message.NewRaw(invalid, message.Attributes{
		message.AttrSpecVersion: "1.0",
		message.AttrType:        "create.order",
		message.AttrSource:      "/test",
		message.AttrID:          uuid.NewString(),
	}, nil)

	// Shutdown
	close(input)
	cancel()
	<-done
}
