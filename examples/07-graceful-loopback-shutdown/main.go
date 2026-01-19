// Example: Graceful loopback shutdown with orchestrated multi-step pipeline.
//
// Demonstrates graceful shutdown of a pipeline with multiple loopback steps:
// - Step 1: Order Received -> Validate Order (loopback)
// - Step 2: Validate Order -> Reserve Inventory (loopback)
// - Step 3: Reserve Inventory -> Process Payment (loopback)
// - Step 4: Process Payment -> Ship Order (loopback)
// - Step 5: Ship Order -> Order Completed (output)
//
// The engine tracks in-flight messages and waits for the pipeline to drain
// before closing loopback outputs. This ensures all messages complete their
// journey through the pipeline without deadlock.
//
// Run: go run ./examples/07-graceful-loopback-shutdown
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/plugin"
	"github.com/google/uuid"
)

// Order flow events
type OrderReceived struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

type OrderValidated struct {
	OrderID string `json:"order_id"`
	Valid   bool   `json:"valid"`
}

type InventoryReserved struct {
	OrderID   string `json:"order_id"`
	Reserved  bool   `json:"reserved"`
	Warehouse string `json:"warehouse"`
}

type PaymentProcessed struct {
	OrderID       string `json:"order_id"`
	Paid          bool   `json:"paid"`
	TransactionID string `json:"transaction_id"`
}

type OrderShipped struct {
	OrderID    string `json:"order_id"`
	TrackingID string `json:"tracking_id"`
}

type OrderCompleted struct {
	OrderID    string `json:"order_id"`
	Status     string `json:"status"`
	TrackingID string `json:"tracking_id"`
}

// typeMatcher matches messages by CloudEvents type attribute.
type typeMatcher struct {
	pattern string
}

func (m *typeMatcher) Match(attrs message.Attributes) bool {
	t, _ := attrs["type"].(string)
	return t == m.pattern
}

func main() {
	// Create engine with a shutdown timeout to ensure graceful drain
	engine := message.NewEngine(message.EngineConfig{
		Marshaler:       message.NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second, // Wait up to 5s for pipeline to drain
	})

	// Step 1: Order Received -> Order Validated
	engine.AddHandler("validate-order", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd OrderReceived) ([]OrderValidated, error) {
			fmt.Printf("[Step 1] Validating order %s (amount: $%.2f)\n", cmd.OrderID, cmd.Amount)
			return []OrderValidated{{
				OrderID: cmd.OrderID,
				Valid:   cmd.Amount > 0,
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
	))

	// Step 2: Order Validated -> Inventory Reserved
	engine.AddHandler("reserve-inventory", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd OrderValidated) ([]InventoryReserved, error) {
			fmt.Printf("[Step 2] Reserving inventory for order %s\n", cmd.OrderID)
			if !cmd.Valid {
				return nil, fmt.Errorf("invalid order")
			}
			return []InventoryReserved{{
				OrderID:   cmd.OrderID,
				Reserved:  true,
				Warehouse: "WH-NYC",
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/inventory", Naming: message.KebabNaming},
	))

	// Step 3: Inventory Reserved -> Payment Processed
	engine.AddHandler("process-payment", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd InventoryReserved) ([]PaymentProcessed, error) {
			fmt.Printf("[Step 3] Processing payment for order %s (warehouse: %s)\n", cmd.OrderID, cmd.Warehouse)
			if !cmd.Reserved {
				return nil, fmt.Errorf("inventory not reserved")
			}
			return []PaymentProcessed{{
				OrderID:       cmd.OrderID,
				Paid:          true,
				TransactionID: "TXN-" + uuid.NewString()[:8],
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/payments", Naming: message.KebabNaming},
	))

	// Step 4: Payment Processed -> Order Shipped
	engine.AddHandler("ship-order", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd PaymentProcessed) ([]OrderShipped, error) {
			fmt.Printf("[Step 4] Shipping order %s (txn: %s)\n", cmd.OrderID, cmd.TransactionID)
			if !cmd.Paid {
				return nil, fmt.Errorf("payment not completed")
			}
			return []OrderShipped{{
				OrderID:    cmd.OrderID,
				TrackingID: "TRACK-" + uuid.NewString()[:8],
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/shipping", Naming: message.KebabNaming},
	))

	// Step 5: Order Shipped -> Order Completed
	engine.AddHandler("complete-order", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd OrderShipped) ([]OrderCompleted, error) {
			fmt.Printf("[Step 5] Completing order %s (tracking: %s)\n", cmd.OrderID, cmd.TrackingID)
			return []OrderCompleted{{
				OrderID:    cmd.OrderID,
				Status:     "completed",
				TrackingID: cmd.TrackingID,
			}}, nil
		},
		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
	))

	// Configure loopbacks: each intermediate step loops back to continue the pipeline
	// Loopback 1: OrderValidated -> back to engine (for Step 2)
	engine.AddPlugin(plugin.Loopback("validate-loop", &typeMatcher{pattern: "order.validated"}))

	// Loopback 2: InventoryReserved -> back to engine (for Step 3)
	engine.AddPlugin(plugin.Loopback("inventory-loop", &typeMatcher{pattern: "inventory.reserved"}))

	// Loopback 3: PaymentProcessed -> back to engine (for Step 4)
	engine.AddPlugin(plugin.Loopback("payment-loop", &typeMatcher{pattern: "payment.processed"}))

	// Loopback 4: OrderShipped -> back to engine (for Step 5)
	engine.AddPlugin(plugin.Loopback("shipping-loop", &typeMatcher{pattern: "order.shipped"}))

	// External input for orders
	input := make(chan *message.RawMessage, 10)
	engine.AddRawInput("orders", nil, input)

	// External output for completed orders
	output, _ := engine.AddRawOutput("completed", &typeMatcher{pattern: "order.completed"})

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Starting Order Processing Pipeline ===")
	fmt.Println()

	// Process multiple orders
	orders := []OrderReceived{
		{OrderID: "ORD-001", Amount: 99.99},
		{OrderID: "ORD-002", Amount: 149.50},
		{OrderID: "ORD-003", Amount: 75.00},
	}

	for _, order := range orders {
		data, _ := json.Marshal(order)
		input <- message.NewRaw(data, message.Attributes{
			message.AttrSpecVersion: "1.0",
			message.AttrType:        "order.received",
			message.AttrSource:      "/external",
			message.AttrID:          uuid.NewString(),
		}, nil)
	}

	// Collect completed orders
	fmt.Println()
	fmt.Println("=== Completed Orders ===")
	for i := 0; i < len(orders); i++ {
		select {
		case result := <-output:
			var completed OrderCompleted
			json.Unmarshal(result.Data, &completed)
			fmt.Printf("Order %s completed with tracking %s\n", completed.OrderID, completed.TrackingID)
		case <-time.After(5 * time.Second):
			log.Fatal("timeout waiting for completed order")
		}
	}

	// Graceful shutdown demonstration
	fmt.Println()
	fmt.Println("=== Initiating Graceful Shutdown ===")

	// Close input first (best practice)
	close(input)

	// Cancel context - engine will:
	// 1. Signal tracker that no more external inputs are expected
	// 2. Wait for all in-flight messages to drain through loopbacks
	// 3. Close loopback outputs to break cycles
	// 4. Complete shutdown cleanly
	cancel()

	// Wait for engine to stop
	select {
	case <-done:
		fmt.Println("Engine shut down gracefully")
	case <-time.After(10 * time.Second):
		log.Fatal("shutdown timeout")
	}
}
