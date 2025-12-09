package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/cqrs"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/middleware"
)

// Domain types
type CreateOrder struct {
	ID         string
	CustomerID string
	Amount     float64
}

type OrderCreated struct {
	ID         string
	CustomerID string
	Amount     float64
	CreatedAt  time.Time
}

// Logging middleware - logs message processing
func LoggingMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return middleware.NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			subject, _ := msg.Properties.Subject()
			log.Printf("[INFO] Processing message: subject=%s", subject)

			start := time.Now()
			result, err := next()
			duration := time.Since(start)

			if err != nil {
				log.Printf("[ERROR] Failed to process message: subject=%s, error=%v, duration=%v",
					subject, err, duration)
			} else {
				log.Printf("[INFO] Successfully processed message: subject=%s, output_count=%d, duration=%v",
					subject, len(result), duration)
			}

			return result, err
		},
	)
}

// Metrics middleware - simulates recording metrics
func MetricsMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return middleware.NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			start := time.Now()
			result, err := next()
			duration := time.Since(start)

			subject, _ := msg.Properties.Subject()
			status := "success"
			if err != nil {
				status = "error"
			}

			log.Printf("[METRICS] subject=%s status=%s duration=%v output_count=%d",
				subject, status, duration, len(result))

			return result, err
		},
	)
}

// Correlation ID middleware - ensures all messages have correlation IDs
func CorrelationIDMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return middleware.NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			// Ensure correlation ID exists
			if _, ok := msg.Properties.CorrelationID(); !ok {
				corrID := fmt.Sprintf("corr-%d", time.Now().UnixNano())
				msg.Properties[message.PropCorrelationID] = corrID
				log.Printf("[CORRELATION] Generated correlation ID: %s", corrID)
			}

			return next()
		},
	)
}

// Validation middleware - validates message properties
func ValidationMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return middleware.NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			// Validate required properties
			if _, ok := msg.Properties.Subject(); !ok {
				err := fmt.Errorf("validation error: missing subject")
				log.Printf("[VALIDATION] %v", err)
				msg.Nack(err)
				return nil, err
			}

			if len(msg.Payload) == 0 {
				err := fmt.Errorf("validation error: empty payload")
				log.Printf("[VALIDATION] %v", err)
				msg.Nack(err)
				return nil, err
			}

			log.Printf("[VALIDATION] Message validated successfully")
			return next()
		},
	)
}

func main() {
	log.Println("=== Router Middleware Example ===")
	log.Println()

	// Create JSON marshaler with property providers
	marshaler := cqrs.NewJSONCommandMarshaler(
		cqrs.WithType("event"),
		cqrs.WithSubjectFromTypeName(),
	)

	// Create command handler
	createOrderHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			log.Printf("[HANDLER] Processing CreateOrder: ID=%s CustomerID=%s Amount=%.2f",
				cmd.ID, cmd.CustomerID, cmd.Amount)

			// Simulate business logic
			time.Sleep(10 * time.Millisecond)

			return []OrderCreated{{
				ID:         cmd.ID,
				CustomerID: cmd.CustomerID,
				Amount:     cmd.Amount,
				CreatedAt:  time.Now(),
			}}, nil
		},
		marshaler,
		cqrs.Match(
			cqrs.MatchSubject("CreateOrder"),
			cqrs.MatchType("command"),
		),
	)

	// Create router with configuration
	// Middleware execution order: Validation → CorrelationID → MessageCorrelation → Logging → Metrics → Handler
	router := message.NewRouter(
		message.RouterConfig{
			Concurrency: 2,
			Recover:     true,
			Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
				ValidationMiddleware(),
				CorrelationIDMiddleware(),   // Generate correlation ID if missing
				middleware.MessageCorrelation(), // Propagate correlation ID to output messages
				LoggingMiddleware(),
				MetricsMiddleware(),
			},
		},
		createOrderHandler,
	)

	// Create test messages
	commands := []CreateOrder{
		{ID: "order-1", CustomerID: "cust-1", Amount: 100.50},
		{ID: "order-2", CustomerID: "cust-2", Amount: 250.75},
		{ID: "order-3", CustomerID: "cust-3", Amount: 50.00},
	}

	// Marshal commands to messages
	var inputMsgs []*message.Message
	for i, cmd := range commands {
		payload, _ := json.Marshal(cmd)
		props := message.Properties{
			message.PropSubject: "CreateOrder",
			"type":              "command",
		}
		// Only add correlation ID to the first message
		if i == 0 {
			props[message.PropCorrelationID] = "manual-corr-123"
		}
		inputMsgs = append(inputMsgs, message.New(payload, props))
	}

	// Create input channel
	input := channel.FromValues(inputMsgs...)

	// Start router
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("Starting message processing...")
	log.Println()

	output := router.Start(ctx, input)

	// Collect and display output messages
	var results []*message.Message
	for msg := range output {
		results = append(results, msg)
	}

	log.Println()
	log.Printf("=== Processing Complete ===")
	log.Printf("Total input messages: %d", len(inputMsgs))
	log.Printf("Total output messages: %d", len(results))
	log.Println()

	// Display output messages
	for i, msg := range results {
		subject, _ := msg.Properties.Subject()
		corrID, _ := msg.Properties.CorrelationID()

		var evt OrderCreated
		json.Unmarshal(msg.Payload, &evt)

		log.Printf("Output Message #%d:", i+1)
		log.Printf("  Subject: %s", subject)
		log.Printf("  Correlation ID: %s", corrID)
		log.Printf("  Order ID: %s", evt.ID)
		log.Printf("  Customer ID: %s", evt.CustomerID)
		log.Printf("  Amount: %.2f", evt.Amount)
		log.Printf("  Created At: %s", evt.CreatedAt.Format(time.RFC3339))
		log.Println()
	}
}
