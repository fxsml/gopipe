// Example: HTTP service with JSON Schema data contracts.
//
// Demonstrates a CloudEvents HTTP service that validates payloads
// against explicit JSON Schema definitions. This solves the Go zero-value
// dilemma: "Is amount really 0, or was the field missing?"
//
// CloudEvents defines the envelope contract (type, source, id).
// JSON Schema defines the payload data contract (what's inside "data").
// Together they provide a full event-driven API spec.
//
//   - POST /events         — CloudEvents endpoint (binary or structured mode)
//   - GET  /schemas        — JSON Schema catalog for all registered types
//   - GET  /schema/{type}  — Individual JSON Schema by type name
//
// Run:
//
//	go run ./examples/07-validating-marshaler
//
// Valid request (binary mode):
//
//	curl -X POST http://localhost:8080/events \
//	  -H "Content-Type: application/json" \
//	  -H "Ce-Specversion: 1.0" \
//	  -H "Ce-Type: create.order.command" \
//	  -H "Ce-Source: /test" \
//	  -H "Ce-Id: 1" \
//	  -d '{"order_id":"ORD-1","amount":49.99}'
//
// Invalid (missing required field — returns 500):
//
//	curl -X POST http://localhost:8080/events \
//	  -H "Content-Type: application/json" \
//	  -H "Ce-Specversion: 1.0" \
//	  -H "Ce-Type: create.order.command" \
//	  -H "Ce-Source: /test" \
//	  -H "Ce-Id: 2" \
//	  -d '{"order_id":"ORD-2"}'
//
// View schema catalog:
//
//	curl http://localhost:8080/schemas
//
// View individual schema:
//
//	curl http://localhost:8080/schema/CreateOrderCommand
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	cehttp "github.com/fxsml/gopipe/message/http"
	"github.com/fxsml/gopipe/message/jsonschema"
)

// Command schema — inbound data contract.
const createOrderCommandSchema = `{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"type": "object",
	"properties": {
		"order_id": { "type": "string", "minLength": 1 },
		"amount":   { "type": "number", "exclusiveMinimum": 0 }
	},
	"required": ["order_id", "amount"],
	"additionalProperties": false
}`

// Event schema — outbound data contract.
const orderCreatedEventSchema = `{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"type": "object",
	"properties": {
		"order_id": { "type": "string" },
		"status":   { "type": "string" }
	},
	"required": ["order_id", "status"],
	"additionalProperties": false
}`

// CreateOrderCommand is the inbound command payload.
type CreateOrderCommand struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// OrderCreatedEvent is the outbound event payload.
type OrderCreatedEvent struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Marshaler with schemas for both command and event.
	marshaler := jsonschema.NewMarshaler()
	marshaler.MustRegister(CreateOrderCommand{}, createOrderCommandSchema)
	marshaler.MustRegister(OrderCreatedEvent{}, orderCreatedEventSchema)

	engine := message.NewEngine(message.EngineConfig{
		Marshaler: marshaler,
		ErrorHandler: func(msg *message.Message, err error) {
			fmt.Fprintf(os.Stderr, "rejected: %v\n", err)
		},
	})

	// Handler: CreateOrderCommand → OrderCreatedEvent.
	engine.AddHandler("process-order", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrderCommand) ([]OrderCreatedEvent, error) {
			return []OrderCreatedEvent{{
				OrderID: cmd.OrderID,
				Status:  "created",
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
	))

	// Input: CloudEvents via HTTP (handles binary, structured, and batch modes).
	subscriber := cehttp.NewSubscriber(cehttp.SubscriberConfig{})
	events, _ := subscriber.Subscribe(ctx)
	engine.AddRawInput("orders", nil, events)

	// Output: print events to stdout via channel.Sink.
	output, _ := engine.AddOutput("stdout", nil)
	printed := channel.Sink(output, func(msg *message.Message) {
		data, _ := json.Marshal(msg.Data)
		fmt.Printf("[%s] %s\n", msg.Type(), data)
	})

	engineDone, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// HTTP routes: CE endpoint + schema catalog + individual schemas.
	mux := http.NewServeMux()
	mux.Handle("POST /events", subscriber)
	mux.HandleFunc("GET /schemas", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		w.Write(marshaler.Schemas())
	})
	mux.HandleFunc("GET /schema/{type}", func(w http.ResponseWriter, r *http.Request) {
		typeName := r.PathValue("type")
		var schema json.RawMessage

		switch typeName {
		case "CreateOrderCommand":
			schema = marshaler.Schema(CreateOrderCommand{})
		case "OrderCreatedEvent":
			schema = marshaler.Schema(OrderCreatedEvent{})
		default:
			http.Error(w, "unknown type", http.StatusNotFound)
			return
		}

		if schema == nil {
			http.Error(w, "schema not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/schema+json")
		w.Write(schema)
	})

	server := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	fmt.Println("Listening on :8080")
	fmt.Println("  POST /events         — CloudEvents endpoint")
	fmt.Println("  GET  /schemas        — JSON Schema catalog")
	fmt.Println("  GET  /schema/{type}  — Individual schema")
	fmt.Println()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

	<-engineDone
	<-printed
}
