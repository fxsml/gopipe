// Example: HTTP server with CloudEvents structured JSON and message engine.
//
// Demonstrates using the message engine to process CloudEvents received via HTTP
// in structured JSON format (all attributes in the JSON body, no headers).
//
// Run: go run ./examples/message-engine
//
// Test with:
//
//	curl -X POST http://localhost:8080/ \
//	  -H "Content-Type: application/cloudevents+json" \
//	  -d '{
//	    "specversion": "1.0",
//	    "type": "order.created",
//	    "source": "/test",
//	    "id": "123",
//	    "data": {"order_id": "ABC123", "amount": 99.99}
//	  }'
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/fxsml/gopipe/message"
)

// OrderCreated is the input event type.
type OrderCreated struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// OrderProcessed is the output event type.
type OrderProcessed struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	// Create engine
	engine := message.NewEngine(message.EngineConfig{
		Marshaler: message.NewJSONMarshaler(),
	})

	// Register handler
	handler := message.NewCommandHandler(
		func(ctx context.Context, cmd OrderCreated) ([]OrderProcessed, error) {
			log.Printf("Processing order: %s (amount: %.2f)", cmd.OrderID, cmd.Amount)
			return []OrderProcessed{{
				OrderID: cmd.OrderID,
				Status:  "processed",
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/orders-service",
			Naming: message.KebabNaming,
		},
	)
	engine.AddHandler(handler, message.HandlerConfig{Name: "process-order"})

	// Create channel for HTTP request/response flow (raw bytes)
	httpRequests := make(chan *message.RawMessage, 10)

	// Connect channel to engine (raw I/O for HTTP integration)
	engine.AddRawInput(httpRequests, message.RawInputConfig{Name: "http-input"})
	output := engine.AddRawOutput(message.RawOutputConfig{Name: "http-output"})

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// HTTP handler - parse and respond with CloudEvents structured JSON
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		msg, err := message.ParseRawMessage(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to parse CloudEvent: %v", err), http.StatusBadRequest)
			return
		}

		httpRequests <- msg

		select {
		case out := <-output:
			w.Header().Set("Content-Type", "application/cloudevents+json")
			out.WriteTo(w)
		case <-r.Context().Done():
			http.Error(w, "timeout", http.StatusGatewayTimeout)
		}
	})

	log.Println("Server starting on :8080")
	fmt.Println()
	fmt.Println("Test with:")
	fmt.Println(`  curl -X POST http://localhost:8080/ \`)
	fmt.Println(`    -H "Content-Type: application/cloudevents+json" \`)
	fmt.Println(`    -d '{"specversion":"1.0","type":"order.created","source":"/test","id":"123","data":{"order_id":"ABC123","amount":99.99}}'`)

	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for engine to complete
	<-done
	fmt.Println("Engine stopped")
}
