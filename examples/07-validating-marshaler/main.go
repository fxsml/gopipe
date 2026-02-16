// Example: HTTP service with JSON Schema validation using middleware.
//
// Demonstrates manual pipeline composition with jsonschema validation middleware.
// This example shows the low-level pipe primitives instead of using the Engine.
//
// Pipeline:
//   HTTP → UnmarshalPipe+ValidationMW → Router → MarshalPipe+ValidationMW → stdout
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
// Invalid (missing required field — returns 400):
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
//	curl http://localhost:8080/schema/create.order.command
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/fxsml/gopipe/message"
	cehttp "github.com/fxsml/gopipe/message/http"
	"github.com/fxsml/gopipe/message/jsonschema"
	"github.com/fxsml/gopipe/pipe"
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

	// 1. Setup JSON Schema registry with validation for both types.
	registry := jsonschema.NewRegistry(jsonschema.Config{
		Naming: message.KebabNaming,
	})
	registry.MustRegisterType(CreateOrderCommand{}, createOrderCommandSchema)
	registry.MustRegisterType(OrderCreatedEvent{}, orderCreatedEventSchema)

	// 2. Setup HTTP CloudEvents subscriber.
	subscriber := cehttp.NewSubscriber(cehttp.SubscriberConfig{})
	rawInput, _ := subscriber.Subscribe(ctx)

	// 3. Unmarshal pipe: RawMessage → Message (with validation middleware).
	marshaler := message.NewJSONMarshaler()
	unmarshalPipe := message.NewUnmarshalPipe(registry, marshaler, message.PipeConfig{})
	unmarshalPipe.Use(jsonschema.NewInputValidationMiddleware(registry))
	typed, _ := unmarshalPipe.Pipe(ctx, rawInput)

	// 4. Router: handle messages and produce new messages.
	router := message.NewRouter(message.PipeConfig{})
	router.AddHandler("process-order", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrderCommand) ([]OrderCreatedEvent, error) {
			log.Printf("Processing order: %s ($%.2f)", cmd.OrderID, cmd.Amount)
			return []OrderCreatedEvent{{
				OrderID: cmd.OrderID,
				Status:  "created",
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
	))
	processed, _ := router.Pipe(ctx, typed)

	// 5. Marshal pipe: Message → RawMessage (with validation middleware).
	marshalPipe := message.NewMarshalPipe(marshaler, message.PipeConfig{})
	marshalPipe.Use(jsonschema.NewOutputValidationMiddleware(registry))
	rawOutput, _ := marshalPipe.Pipe(ctx, processed)

	// 6. Sink to stdout using pipe primitive.
	printer := pipe.NewSinkPipe(func(ctx context.Context, raw *message.RawMessage) error {
		var data any
		json.Unmarshal(raw.Data, &data)
		formatted, _ := json.MarshalIndent(data, "", "  ")
		fmt.Printf("\n[%s]\n%s\n", raw.Type(), formatted)
		return nil
	}, pipe.Config{})
	done, _ := printer.Pipe(ctx, rawOutput)

	// 7. HTTP routes: CloudEvents endpoint + schema discovery.
	mux := http.NewServeMux()
	mux.Handle("POST /events", subscriber)
	mux.HandleFunc("GET /schemas", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		w.Write(registry.Schemas())
	})
	mux.HandleFunc("GET /schema/{type}", func(w http.ResponseWriter, r *http.Request) {
		eventType := r.PathValue("type")
		schema := registry.Schema(eventType)

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
	fmt.Println("Pipeline: HTTP → Unmarshal → Router → Marshal → stdout")
	fmt.Println()
	fmt.Println("Try: curl -X POST http://localhost:8080/events \\")
	fmt.Println(`       -H "Ce-Specversion: 1.0" -H "Ce-Id: 1" \`)
	fmt.Println(`       -H "Ce-Type: create.order.command" -H "Ce-Source: /test" \`)
	fmt.Println(`       -d '{"order_id":"ORD-1","amount":49.99}'`)
	fmt.Println()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

	<-done
}
