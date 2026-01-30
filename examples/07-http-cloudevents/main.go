// Example: HTTP CloudEvents Adapter
//
// Demonstrates the HTTP CloudEvents adapter with:
// - Topic-based subscription via http.ServeMux
// - Batch publishing for high throughput
// - Standard library http.Client/Server
//
// Run:
//
//	go run ./examples/07-http-cloudevents
//
// Then send events:
//
//	curl -X POST http://localhost:8080/events/orders \
//	  -H "Content-Type: application/cloudevents+json" \
//	  -d '{"specversion":"1.0","id":"1","type":"order.created","source":"/shop","data":{"order_id":"ORD-001","amount":100}}'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fxsml/gopipe/message"
	cehttp "github.com/fxsml/gopipe/message/cloudevents/http"
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
	// SUBSCRIBER: One per topic, compose with http.ServeMux
	// ─────────────────────────────────────────────────────────────────────

	orders := cehttp.NewSubscriber(cehttp.SubscriberConfig{BufferSize: 100})

	mux := http.NewServeMux()
	mux.Handle("/events/orders", orders)

	// ─────────────────────────────────────────────────────────────────────
	// PUBLISHER: Send confirmations with batching
	// ─────────────────────────────────────────────────────────────────────

	pub := cehttp.NewPublisher(cehttp.PublisherConfig{
		TargetURL:   "http://localhost:8081/events", // Would be external service
		Concurrency: 2,
	})

	confirmationsCh := make(chan *message.RawMessage, 100)

	// Start batch publisher (collects up to 10 or flushes every 500ms)
	_, err := pub.PublishBatch(ctx, "confirmations", confirmationsCh, cehttp.BatchConfig{
		MaxSize:     10,
		MaxDuration: 500 * time.Millisecond,
	})
	if err != nil {
		log.Fatalf("Failed to start batch publisher: %v", err)
	}

	// ─────────────────────────────────────────────────────────────────────
	// PROCESSOR: Handle orders and emit confirmations
	// ─────────────────────────────────────────────────────────────────────

	go func() {
		for raw := range orders.C() {
			var order Order
			if err := json.Unmarshal(raw.Data, &order); err != nil {
				log.Printf("Failed to parse order: %v", err)
				raw.Nack(err)
				continue
			}

			fmt.Printf("Received order: %s (amount: %d)\n", order.OrderID, order.Amount)

			confirmation := OrderConfirmation{
				OrderID:   order.OrderID,
				Status:    "confirmed",
				Confirmed: time.Now(),
			}

			data, _ := json.Marshal(confirmation)
			confirmationMsg := message.NewRaw(data, message.Attributes{
				message.AttrID:     fmt.Sprintf("conf-%s", order.OrderID),
				message.AttrType:   "order.confirmed",
				message.AttrSource: "/processor",
			}, nil)

			select {
			case confirmationsCh <- confirmationMsg:
			default:
				log.Println("Confirmation channel full, dropping")
			}

			raw.Ack()
		}
		close(confirmationsCh)
	}()

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
	fmt.Println("Topics available:")
	fmt.Println("  POST /events/orders - Submit orders")
	fmt.Println()
	fmt.Println("Send a test event:")
	fmt.Println()
	fmt.Println(`curl -X POST http://localhost:8080/events/orders \`)
	fmt.Println(`  -H "Content-Type: application/cloudevents+json" \`)
	fmt.Println(`  -d '{"specversion":"1.0","id":"1","type":"order.created","source":"/shop","data":{"order_id":"ORD-001","amount":100}}'`)
	fmt.Println()
	fmt.Println("Or send a batch:")
	fmt.Println()
	fmt.Println(`curl -X POST http://localhost:8080/events/orders \`)
	fmt.Println(`  -H "Content-Type: application/cloudevents-batch+json" \`)
	fmt.Println(`  -d '[{"specversion":"1.0","id":"1","type":"order.created","source":"/shop","data":{"order_id":"ORD-001","amount":100}},{"specversion":"1.0","id":"2","type":"order.created","source":"/shop","data":{"order_id":"ORD-002","amount":200}}]'`)
	fmt.Println()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	fmt.Println("Shutting down...")
}
