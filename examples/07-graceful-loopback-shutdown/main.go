// Example: Graceful loopback shutdown with batching (fan-in).
//
// Demonstrates graceful shutdown with BatchLoopback that collapses multiple
// messages into one (fan-in). The engine correctly tracks in-flight messages
// even when N inputs become M outputs (where M < N).
//
// Pipeline:
//   - Orders enter individually
//   - BatchLoopback collects orders and produces a single batch reservation
//   - Batch is processed and individual confirmations are sent
//
// Key insight: Without proper tracking, 10 orders batched into 1 would leave
// the tracker stuck at count=9, causing shutdown to timeout. The engine's
// AdjustInFlight mechanism ensures the tracker stays balanced.
//
// Run: go run ./examples/07-graceful-loopback-shutdown
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/match"
	"github.com/fxsml/gopipe/message/plugin"
)

// Order represents an incoming order.
type Order struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// BatchReservation represents a batched inventory check for multiple orders.
type BatchReservation struct {
	OrderIDs  []string `json:"order_ids"`
	Warehouse string   `json:"warehouse"`
}

// OrderConfirmed is the final output for each order.
type OrderConfirmed struct {
	OrderID   string `json:"order_id"`
	Warehouse string `json:"warehouse"`
	Status    string `json:"status"`
}

func main() {
	engine := message.NewEngine(message.EngineConfig{
		Marshaler:       message.NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
	})

	// Step 1: Receive orders (pass-through to loopback)
	engine.AddHandler("receive-order", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd Order) ([]Order, error) {
			fmt.Printf("[Receive] Order %s ($%.2f)\n", cmd.OrderID, cmd.Amount)
			return []Order{cmd}, nil
		},
		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
	))

	// Step 2: Process batch reservation -> fan-out to individual confirmations
	engine.AddHandler("process-batch", nil, message.NewCommandHandler(
		func(ctx context.Context, cmd BatchReservation) ([]OrderConfirmed, error) {
			fmt.Printf("[Batch] Processing %d orders for %s\n", len(cmd.OrderIDs), cmd.Warehouse)

			// Fan-out: one batch -> multiple confirmations
			var confirmations []OrderConfirmed
			for _, orderID := range cmd.OrderIDs {
				confirmations = append(confirmations, OrderConfirmed{
					OrderID:   orderID,
					Warehouse: cmd.Warehouse,
					Status:    "confirmed",
				})
			}
			return confirmations, nil
		},
		message.CommandHandlerConfig{Source: "/inventory", Naming: message.KebabNaming},
	))

	// BatchLoopback: Collect orders and batch them (fan-in: N orders -> 1 batch)
	// This is where the tracker adjustment is critical for graceful shutdown.
	engine.AddPlugin(plugin.BatchLoopback(
		"order-batcher",
		match.Types("order"),
		func(msgs []*message.Message) []*message.Message {
			var orderIDs []string
			for _, m := range msgs {
				order := m.Data.(Order)
				orderIDs = append(orderIDs, order.OrderID)
			}
			fmt.Printf("[BatchLoopback] Batching %d orders: %s\n", len(orderIDs), strings.Join(orderIDs, ", "))

			// Fan-in: N orders -> 1 batch reservation
			return []*message.Message{{
				Data:       BatchReservation{OrderIDs: orderIDs, Warehouse: "WH-CENTRAL"},
				Attributes: message.Attributes{"type": "batch.reservation"},
			}}
		},
		plugin.BatchLoopbackConfig{
			MaxSize:     5,                    // Batch up to 5 orders
			MaxDuration: 200 * time.Millisecond, // Or flush after 200ms
		},
	))

	// Setup I/O using channel utilities
	// FromRange(1,11) produces 1..10, Transform converts to RawMessage
	input := channel.Transform(channel.FromRange(1, 11), func(i int) *message.RawMessage {
		order := Order{OrderID: fmt.Sprintf("ORD-%03d", i), Amount: float64(i) * 10}
		data, _ := json.Marshal(order)
		return message.NewRaw(data, message.Attributes{"type": "order"}, nil)
	})
	engine.AddRawInput("orders", nil, input)
	output, _ := engine.AddRawOutput("confirmed", match.Types("order.confirmed"))

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	done, err := engine.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Order Batching Pipeline (10 orders -> 2 batches -> 10 confirmations) ===")
	fmt.Println()

	// Collect confirmations using Sink (processes in background)
	fmt.Println("=== Confirmations ===")
	const expectedConfirmations = 10
	var count atomic.Int32
	allReceived := make(chan struct{})

	channel.Sink(output, func(raw *message.RawMessage) {
		var confirmed OrderConfirmed
		json.Unmarshal(raw.Data, &confirmed)
		fmt.Printf("✓ %s -> %s (%s)\n", confirmed.OrderID, confirmed.Warehouse, confirmed.Status)
		if count.Add(1) == expectedConfirmations {
			close(allReceived)
		}
	})

	// Wait for all confirmations before initiating shutdown
	<-allReceived
	fmt.Printf("\nReceived %d confirmations\n", count.Load())

	// Graceful shutdown - should complete quickly, not wait for timeout
	fmt.Println()
	fmt.Println("=== Graceful Shutdown ===")
	shutdownStart := time.Now()
	cancel()

	select {
	case <-done:
		elapsed := time.Since(shutdownStart)
		fmt.Printf("Shutdown completed in %v\n", elapsed)
		if elapsed < time.Second {
			fmt.Println("✓ Tracker correctly adjusted for fan-in (no timeout)")
		} else {
			fmt.Println("✗ Shutdown took too long - tracker may be stuck")
		}
	case <-time.After(10 * time.Second):
		log.Fatal("shutdown timeout - tracker stuck!")
	}
}
