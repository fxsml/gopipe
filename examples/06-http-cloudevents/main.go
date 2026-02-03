// Example: HTTP CloudEvents Adapter
//
// Demonstrates HTTP pub/sub using the message/http adapter:
// - Receive CloudEvents via HTTP (binary or structured mode)
// - Process with typed handlers
// - Publish results via HTTP (batched for efficiency)
//
// Run:
//
//	go run ./examples/06-http-cloudevents
//
// Send events (binary mode - default, efficient):
//
//	curl -X POST http://localhost:8080/events/orders \
//	  -H "Content-Type: application/json" \
//	  -H "Ce-Specversion: 1.0" \
//	  -H "Ce-Id: 1" \
//	  -H "Ce-Type: order.created" \
//	  -H "Ce-Source: /shop" \
//	  -d '{"order_id":"ORD-001","amount":100}'
//
// Or structured mode (metadata in JSON):
//
//	curl -X POST http://localhost:8080/events/orders \
//	  -H "Content-Type: application/cloudevents+json" \
//	  -d '{"specversion":"1.0","id":"1","type":"order.created","source":"/shop","data":{"order_id":"ORD-001","amount":100}}'
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

type OrderCreated struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

type OrderConfirmed struct {
	OrderID   string    `json:"order_id"`
	Status    string    `json:"status"`
	Confirmed time.Time `json:"confirmed_at"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Setup engine with handler: OrderCreated → OrderConfirmed
	engine := message.NewEngine(message.EngineConfig{})
	engine.AddHandler("process-order", nil, message.NewCommandHandler(
		func(ctx context.Context, order OrderCreated) ([]OrderConfirmed, error) {
			fmt.Printf("Processing: %s ($%d)\n", order.OrderID, order.Amount)
			return []OrderConfirmed{{
				OrderID:   order.OrderID,
				Status:    "confirmed",
				Confirmed: time.Now(),
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/processor",
			Naming: message.KebabNaming,
		},
	))

	// HTTP Subscriber: receive orders
	orders := cehttp.NewSubscriber(cehttp.SubscriberConfig{BufferSize: 100})
	ordersCh, _ := orders.Subscribe(ctx)
	engine.AddRawInput("orders", nil, ordersCh)

	// HTTP Publisher: send confirmations (batched)
	confirmationsCh, _ := engine.AddRawOutput("confirmations", nil)
	publisher := cehttp.NewPublisher(cehttp.PublisherConfig{
		TargetURL:     "http://localhost:9000/confirmations", // External service
		BatchSize:     10,
		BatchDuration: time.Second,
		ErrorHandler: func(msg *message.RawMessage, err error) {
			log.Printf("Send failed (id=%v): %v", msg.ID(), err)
		},
	})
	publisher.Publish(ctx, confirmationsCh)

	// Start engine
	engineDone, _ := engine.Start(ctx)

	// HTTP Server (receives orders)
	mux := http.NewServeMux()
	mux.Handle("POST /events/orders", orders)

	server := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	// Echo server (receives confirmations)
	echoMux := http.NewServeMux()
	echoMux.HandleFunc("POST /confirmations", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		fmt.Printf("→ Confirmation: %s\n", body)
		w.WriteHeader(http.StatusOK)
	})

	echoServer := &http.Server{Addr: ":9000", Handler: echoMux}
	go func() {
		<-ctx.Done()
		echoServer.Shutdown(context.Background())
	}()
	go echoServer.ListenAndServe()

	fmt.Println("Order receiver:        :8080")
	fmt.Println("Confirmation receiver: :9000")
	fmt.Println()
	fmt.Println("Try: curl -X POST http://localhost:8080/events/orders \\")
	fmt.Println(`       -H "Ce-Specversion: 1.0" -H "Ce-Id: 1" \`)
	fmt.Println(`       -H "Ce-Type: order.created" -H "Ce-Source: /shop" \`)
	fmt.Println(`       -d '{"order_id":"ORD-001","amount":100}'`)
	fmt.Println()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	<-engineDone
	fmt.Println("Shutdown complete")
}
