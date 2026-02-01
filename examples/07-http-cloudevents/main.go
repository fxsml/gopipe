// Example: HTTP CloudEvents Adapter with Engine
//
// Demonstrates the HTTP CloudEvents adapter integrated with the message engine:
// - HTTP Subscriber as raw input
// - Command handler for processing
// - HTTP Publisher as raw output
// - Standard library http.Client/Server
//
// Run:
//
//	go run ./examples/07-http-cloudevents
//
// Then send events (structured mode):
//
//	curl -X POST http://localhost:8080/events/orders \
//	  -H "Content-Type: application/cloudevents+json" \
//	  -d '{"specversion":"1.0","id":"1","type":"order.created","source":"/shop","data":{"order_id":"ORD-001","amount":100}}'
//
// Or send events (binary mode):
//
//	curl -X POST http://localhost:8080/events/orders \
//	  -H "Content-Type: application/json" \
//	  -H "Ce-Specversion: 1.0" \
//	  -H "Ce-Id: 1" \
//	  -H "Ce-Type: order.created" \
//	  -H "Ce-Source: /shop" \
//	  -d '{"order_id":"ORD-001","amount":100}'
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fxsml/gopipe/message"
	cehttp "github.com/fxsml/gopipe/message/http"
)

type Order struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

type OrderConfirmation struct {
	OrderID   string    `json:"order_id"`
	Status    string    `json:"status"`
	Confirmed time.Time `json:"confirmed_at"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// ─────────────────────────────────────────────────────────────────────
	// ENGINE: Orchestrates message flow
	// ─────────────────────────────────────────────────────────────────────

	engine := message.NewEngine(message.EngineConfig{})

	// Register handler: Order → OrderConfirmation
	handler := message.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmation, error) {
			fmt.Printf("Received order: %s (amount: %d)\n", order.OrderID, order.Amount)
			return []OrderConfirmation{{
				OrderID:   order.OrderID,
				Status:    "confirmed",
				Confirmed: time.Now(),
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/processor",
			Naming: message.KebabNaming,
		},
	)
	engine.AddHandler("process-order", nil, handler)

	// ─────────────────────────────────────────────────────────────────────
	// SUBSCRIBER: HTTP input → engine
	// ─────────────────────────────────────────────────────────────────────

	orders := cehttp.NewSubscriber(cehttp.SubscriberConfig{BufferSize: 100})
	ordersCh, err := orders.Subscribe(ctx)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Connect HTTP subscriber to engine
	engine.AddRawInput("orders", nil, ordersCh)

	// Go 1.22+ routing with method matching
	mux := http.NewServeMux()
	mux.Handle("POST /events/orders", orders)

	// ─────────────────────────────────────────────────────────────────────
	// PUBLISHER: Engine output → HTTP
	// ─────────────────────────────────────────────────────────────────────

	// Get raw output from engine
	confirmationsCh, err := engine.AddRawOutput("confirmations", nil)
	if err != nil {
		log.Fatalf("Failed to add output: %v", err)
	}

	// Publisher with batch mode for high throughput
	confirmationsPub := cehttp.NewPublisher(cehttp.PublisherConfig{
		TargetURL:     "http://localhost:8081/events/confirmations",
		Concurrency:   2,
		BatchSize:     10,
		BatchDuration: 500 * time.Millisecond,
	})

	// Connect engine output to HTTP publisher
	_, err = confirmationsPub.Publish(ctx, confirmationsCh)
	if err != nil {
		log.Fatalf("Failed to start publisher: %v", err)
	}

	// Start engine
	engineDone, err := engine.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start engine: %v", err)
	}

	// ─────────────────────────────────────────────────────────────────────
	// START SERVER (standard library)
	// ─────────────────────────────────────────────────────────────────────

	server := &http.Server{Addr: ":8080", Handler: mux}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	fmt.Println("HTTP CloudEvents server listening on :8080")
	fmt.Println()
	fmt.Println("Send a test event (structured mode):")
	fmt.Println()
	fmt.Println(`  curl -X POST http://localhost:8080/events/orders \`)
	fmt.Println(`    -H "Content-Type: application/cloudevents+json" \`)
	fmt.Println(`    -d '{"specversion":"1.0","id":"1","type":"order.created","source":"/shop","data":{"order_id":"ORD-001","amount":100}}'`)
	fmt.Println()
	fmt.Println("Or binary mode:")
	fmt.Println()
	fmt.Println(`  curl -X POST http://localhost:8080/events/orders \`)
	fmt.Println(`    -H "Content-Type: application/json" \`)
	fmt.Println(`    -H "Ce-Specversion: 1.0" -H "Ce-Id: 1" -H "Ce-Type: order.created" -H "Ce-Source: /shop" \`)
	fmt.Println(`    -d '{"order_id":"ORD-001","amount":100}'`)
	fmt.Println()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	<-engineDone
	fmt.Println("Shutdown complete")
}
