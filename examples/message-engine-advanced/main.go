// Example: Advanced message engine with multiple inputs, outputs, and loopback.
//
// This example demonstrates:
// - Multiple input channels (orders, payments)
// - Multiple output channels with routing based on event type
// - Loopback for internal event re-processing (saga pattern)
// - Using channel package helpers
//
// Run: go run ./examples/message-engine-advanced
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/match"
)

// --- Commands (inputs) ---

type CreateOrder struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
}

type ProcessPayment struct {
	PaymentID string  `json:"payment_id"`
	OrderID   string  `json:"order_id"`
	Amount    float64 `json:"amount"`
}

// --- Events (outputs) ---

type OrderCreated struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
}

type PaymentProcessed struct {
	PaymentID string `json:"payment_id"`
	OrderID   string `json:"order_id"`
	Status    string `json:"status"`
}

type OrderCompleted struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	// Create engine with custom error handler
	engine := message.NewEngine(message.EngineConfig{
		Marshaler: message.NewJSONMarshaler(),
		ErrorHandler: func(msg *message.Message, err error) {
			log.Printf("ERROR: %v (type: %v)", err, msg.Attributes["type"])
		},
	})

	// --- Register Handlers ---

	// Handler 1: CreateOrder -> OrderCreated
	orderHandler := message.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			log.Printf("Creating order: %s for customer %s", cmd.OrderID, cmd.CustomerID)
			return []OrderCreated{{
				OrderID:    cmd.OrderID,
				CustomerID: cmd.CustomerID,
				Amount:     cmd.Amount,
				Status:     "created",
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/order-service",
			Naming: message.KebabNaming,
		},
	)
	engine.AddHandler(orderHandler, message.HandlerConfig{Name: "create-order"})

	// Handler 2: ProcessPayment -> PaymentProcessed
	paymentHandler := message.NewCommandHandler(
		func(ctx context.Context, cmd ProcessPayment) ([]PaymentProcessed, error) {
			log.Printf("Processing payment: %s for order %s", cmd.PaymentID, cmd.OrderID)
			return []PaymentProcessed{{
				PaymentID: cmd.PaymentID,
				OrderID:   cmd.OrderID,
				Status:    "completed",
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/payment-service",
			Naming: message.KebabNaming,
		},
	)
	engine.AddHandler(paymentHandler, message.HandlerConfig{Name: "process-payment"})

	// Handler 3: OrderCreated -> OrderCompleted (saga step - triggered via loopback)
	completionHandler := message.NewCommandHandler(
		func(ctx context.Context, event OrderCreated) ([]OrderCompleted, error) {
			log.Printf("Completing order: %s (saga continuation)", event.OrderID)
			return []OrderCompleted{{
				OrderID: event.OrderID,
				Status:  "completed",
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/saga-orchestrator",
			Naming: message.KebabNaming,
		},
	)
	engine.AddHandler(completionHandler, message.HandlerConfig{Name: "complete-order"})

	// --- Configure Inputs (Raw I/O for broker integration) ---

	// Input 1: Order commands
	orderInput := make(chan *message.RawMessage, 10)
	engine.AddRawInput(orderInput, message.RawInputConfig{
		Name:    "order-commands",
		Matcher: match.Types("create.order"),
	})

	// Input 2: Payment commands
	paymentInput := make(chan *message.RawMessage, 10)
	engine.AddRawInput(paymentInput, message.RawInputConfig{
		Name:    "payment-commands",
		Matcher: match.Types("process.payment"),
	})

	// --- Configure Loopback ---

	// Loopback: Route OrderCreated events back for saga continuation
	engine.AddLoopback(message.LoopbackConfig{
		Name:    "saga-loopback",
		Matcher: match.Types("order.created"),
	})

	// --- Configure Outputs (Raw I/O for broker integration) ---

	// Output 1: Order events (order.*)
	orderOutput := engine.AddRawOutput(message.RawOutputConfig{
		Name:    "order-events",
		Matcher: match.Types("order.%"),
	})

	// Output 2: Payment events (payment.*)
	paymentOutput := engine.AddRawOutput(message.RawOutputConfig{
		Name:    "payment-events",
		Matcher: match.Types("payment.%"),
	})

	// --- Start Engine ---

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// --- Consume Outputs ---

	// Use channel.Sink to consume order events
	orderSinkDone := channel.Sink(orderOutput, func(raw *message.RawMessage) {
		log.Printf("ORDER OUTPUT: type=%s data=%s", raw.Attributes["type"], raw.Data)
	})

	// Use channel.Sink to consume payment events
	paymentSinkDone := channel.Sink(paymentOutput, func(raw *message.RawMessage) {
		log.Printf("PAYMENT OUTPUT: type=%s data=%s", raw.Attributes["type"], raw.Data)
	})

	// --- Simulate Input Messages Using channel.FromSlice ---

	// Create test order commands using channel.FromSlice
	orderData, _ := json.Marshal(CreateOrder{
		OrderID:    "ORD-001",
		CustomerID: "CUST-123",
		Amount:     99.99,
	})
	orderCommands := channel.FromSlice([]*message.RawMessage{{
		Data: orderData,
		Attributes: message.Attributes{
			"type":   "create.order",
			"source": "/api",
			"id":     "msg-1",
		},
	}})

	// Create test payment commands using channel.FromSlice
	paymentData, _ := json.Marshal(ProcessPayment{
		PaymentID: "PAY-001",
		OrderID:   "ORD-001",
		Amount:    99.99,
	})
	paymentCommands := channel.FromSlice([]*message.RawMessage{{
		Data: paymentData,
		Attributes: message.Attributes{
			"type":   "process.payment",
			"source": "/api",
			"id":     "msg-2",
		},
	}})

	// Use channel.Sink to forward test messages to engine inputs
	// This demonstrates using Sink for message forwarding
	go func() {
		time.Sleep(100 * time.Millisecond)
		<-channel.Sink(orderCommands, func(msg *message.RawMessage) {
			orderInput <- msg
		})
		close(orderInput)
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		<-channel.Sink(paymentCommands, func(msg *message.RawMessage) {
			paymentInput <- msg
		})
		close(paymentInput)
	}()

	// --- Wait for Completion ---

	// Merge done channels using channel.Merge and wait for all to complete
	allDone := channel.Merge(done, orderSinkDone, paymentSinkDone)
	<-channel.Drain(allDone)

	fmt.Println("\n=== Summary ===")
	fmt.Println("Demonstrated:")
	fmt.Println("  - Multiple input channels (orders, payments)")
	fmt.Println("  - Input matchers filtering by event type")
	fmt.Println("  - Multiple output channels with type-based routing")
	fmt.Println("  - Loopback for saga pattern (OrderCreated -> OrderCompleted)")
	fmt.Println("  - channel.FromSlice for generating test messages")
	fmt.Println("  - channel.Sink for consuming outputs and forwarding messages")
	fmt.Println("  - channel.Merge and channel.Drain for coordinating shutdown")
}
