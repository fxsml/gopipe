// Example: HTTP CloudEvents Adapter
//
// Demonstrates the HTTP CloudEvents adapter with:
// - Topic-based subscription (multiple endpoints)
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
	// SUBSCRIBER: Receive events on topic-based endpoints
	// ─────────────────────────────────────────────────────────────────────

	sub := cehttp.NewSubscriber(cehttp.SubscriberConfig{
		Addr:       ":8080",
		Path:       "/events",
		BufferSize: 100,
	})

	// Subscribe to orders topic (/events/orders)
	ordersCh, err := sub.Subscribe(ctx, "orders")
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// ─────────────────────────────────────────────────────────────────────
	// PUBLISHER: Send confirmations with batching
	// ─────────────────────────────────────────────────────────────────────

	pub := cehttp.NewPublisher(cehttp.PublisherConfig{
		TargetURL:   "http://localhost:8081/events", // Would be external service
		Concurrency: 2,
	})

	// Channel for outbound confirmations
	confirmationsCh := make(chan *message.RawMessage, 100)

	// Start batch publisher (collects up to 10 or flushes every 500ms)
	// Note: This will fail to connect since there's no server on 8081,
	// but demonstrates the API usage
	_, err = pub.PublishBatch(ctx, "confirmations", confirmationsCh, cehttp.BatchConfig{
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
		for raw := range ordersCh {
			// Parse order data
			var order Order
			if err := json.Unmarshal(raw.Data, &order); err != nil {
				log.Printf("Failed to parse order: %v", err)
				raw.Nack(err)
				continue
			}

			fmt.Printf("Received order: %s (amount: %d)\n", order.OrderID, order.Amount)

			// Create confirmation (would be sent to external service)
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

			// Queue for batch publishing
			select {
			case confirmationsCh <- confirmationMsg:
			default:
				log.Println("Confirmation channel full, dropping")
			}

			// Acknowledge the order
			raw.Ack()
		}
		close(confirmationsCh)
	}()

	// ─────────────────────────────────────────────────────────────────────
	// START SERVER
	// ─────────────────────────────────────────────────────────────────────

	go func() {
		if err := sub.Start(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Server error: %v", err)
		}
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

	<-ctx.Done()
	fmt.Println("Shutting down...")
}
