// Example: Minimal HTTP server with CloudEvents and message engine.
//
// Run: go run ./example
//
// Test with:
//
//	curl -X POST http://localhost:8080/ \
//	  -H "Content-Type: application/json" \
//	  -H "Ce-Id: 123" \
//	  -H "Ce-Type: order.created" \
//	  -H "Ce-Source: /test" \
//	  -H "Ce-Specversion: 1.0" \
//	  -d '{"order_id": "ABC123", "amount": 99.99}'
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
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

	// Create input/output channels
	input := make(chan *message.RawMessage, 10)
	engine.AddInput(input, message.InputConfig{Name: "http-input"})
	output := engine.AddOutput(message.OutputConfig{Name: "http-output"})

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// HTTP handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Parse CloudEvent from HTTP request
		event, err := cloudevents.NewEventFromHTTPRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Convert to RawMessage
		raw := &message.RawMessage{
			Data: event.Data(),
			Attributes: message.Attributes{
				"id":          event.ID(),
				"type":        event.Type(),
				"source":      event.Source(),
				"specversion": event.SpecVersion(),
			},
		}

		// Send to engine
		input <- raw

		// Wait for output
		select {
		case out := <-output:
			w.Header().Set("Content-Type", "application/json")
			w.Write(out.Data)
		case <-r.Context().Done():
			http.Error(w, "timeout", http.StatusGatewayTimeout)
		}
	})

	log.Println("Server starting on :8080")
	log.Println("Test with:")
	log.Println(`  curl -X POST http://localhost:8080/ \`)
	log.Println(`    -H "Content-Type: application/json" \`)
	log.Println(`    -H "Ce-Id: 123" \`)
	log.Println(`    -H "Ce-Type: order.created" \`)
	log.Println(`    -H "Ce-Source: /test" \`)
	log.Println(`    -H "Ce-Specversion: 1.0" \`)
	log.Println(`    -d '{"order_id": "ABC123", "amount": 99.99}'`)

	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for engine to complete
	<-done
	fmt.Println("Engine stopped")
}
