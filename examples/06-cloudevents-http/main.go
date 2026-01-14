// Example: CloudEvents over HTTP
//
// This example demonstrates the CloudEvents integration using HTTP protocol.
// It starts an HTTP server that receives CloudEvents, processes them through
// the gopipe engine, and prints the results.
//
// Run:
//
//	go run ./examples/06-cloudevents-http
//
// Then send events:
//
//	curl -X POST http://localhost:8080 \
//	  -H "Ce-Id: test-123" \
//	  -H "Ce-Type: order.created" \
//	  -H "Ce-Source: /test" \
//	  -H "Ce-Specversion: 1.0" \
//	  -H "Content-Type: application/json" \
//	  -d '{"order_id":"ORD-001","amount":100}'
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"

	"github.com/fxsml/gopipe/message"
	ceclient "github.com/fxsml/gopipe/message/cloudevents"
)

// OrderCreated is the input event type.
type OrderCreated struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Create CloudEvents HTTP protocol (receiver)
	protocol, err := cehttp.New(cehttp.WithPort(8080))
	if err != nil {
		log.Fatalf("Failed to create HTTP protocol: %v", err)
	}

	// Setup gopipe engine
	engine := message.NewEngine(message.EngineConfig{
		Marshaler:  message.NewJSONMarshaler(),
		BufferSize: 10,
	})

	// Register CloudEvents HTTP receiver as input
	err = engine.AddPlugin(
		ceclient.SubscriberPlugin(
			ctx, "http-input", nil,
			protocol, ceclient.SubscriberConfig{},
		),
	)
	if err != nil {
		log.Fatalf("Failed to add plugin: %v", err)
	}

	// Register handler for OrderCreated events
	handler := message.NewCommandHandler(
		func(ctx context.Context, order OrderCreated) ([]struct{}, error) {
			fmt.Printf("Processing order: %s (amount: %d)\n", order.OrderID, order.Amount)
			return nil, nil // no output events
		},
		message.CommandHandlerConfig{
			Source: "/orders",
			Naming: message.KebabNaming,
		},
	)
	if err := engine.AddHandler("order-processor", nil, handler); err != nil {
		log.Fatalf("Failed to add handler: %v", err)
	}

	// Start HTTP server (runs in background)
	go func() {
		if err := protocol.OpenInbound(ctx); err != nil && ctx.Err() == nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start engine
	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start engine: %v", err)
	}

	fmt.Println("Listening on http://localhost:8080")
	fmt.Println()
	fmt.Println("Send a test event:")
	fmt.Println()
	fmt.Println(`curl -X POST http://localhost:8080 \`)
	fmt.Println(`  -H "Ce-Id: test-123" \`)
	fmt.Println(`  -H "Ce-Type: order.created" \`)
	fmt.Println(`  -H "Ce-Source: /test" \`)
	fmt.Println(`  -H "Ce-Specversion: 1.0" \`)
	fmt.Println(`  -H "Content-Type: application/json" \`)
	fmt.Println(`  -d '{"order_id":"ORD-001","amount":100}'`)
	fmt.Println()

	<-done
}
