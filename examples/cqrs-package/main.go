package main

import (
	"context"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message/cqrs"
	"github.com/fxsml/gopipe/message"
)

// createCommand creates a command message from a command struct.
func createCommand(marshaler cqrs.Marshaler, cmd any, attrs message.Attributes) *message.Message {
	payload, err := marshaler.Marshal(cmd)
	if err != nil {
		log.Printf("failed to marshal command: %v", err)
		return nil
	}

	if attrs == nil {
		attrs = message.Attributes{}
	}

	// Extract type name from the command
	t := reflect.TypeOf(cmd)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	attrs[message.AttrSubject] = t.Name()
	attrs[message.AttrType] = t.Name()

	return message.New(payload, attrs)
}

// createCommands creates multiple command messages with optional correlation ID.
func createCommands(marshaler cqrs.Marshaler, correlationID string, cmds ...any) []*message.Message {
	msgs := make([]*message.Message, 0, len(cmds))

	for _, cmd := range cmds {
		attrs := message.Attributes{}
		if correlationID != "" {
			attrs[message.AttrCorrelationID] = correlationID
		}

		msg := createCommand(marshaler, cmd, attrs)
		if msg != nil {
			msgs = append(msgs, msg)
		}
	}

	return msgs
}

// ============================================================================
// Domain: Order Processing
// ============================================================================

// Commands
type CreateOrder struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
}

type ChargePayment struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
}

type ReserveInventory struct {
	OrderID string `json:"order_id"`
	SKU     string `json:"sku"`
}

type ShipOrder struct {
	OrderID string `json:"order_id"`
	Address string `json:"address"`
}

// Events
type OrderCreated struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Amount     int       `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
}

type PaymentCharged struct {
	OrderID   string    `json:"order_id"`
	Amount    int       `json:"amount"`
	ChargedAt time.Time `json:"charged_at"`
}

type InventoryReserved struct {
	OrderID    string    `json:"order_id"`
	ReservedAt time.Time `json:"reserved_at"`
}

type OrderShipped struct {
	OrderID    string    `json:"order_id"`
	TrackingID string    `json:"tracking_id"`
	ShippedAt  time.Time `json:"shipped_at"`
}

// ============================================================================
// Command Handlers (Commands → Events)
// ============================================================================

func handleCreateOrder(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
	log.Printf("   💾 Saving order to database...")
	return []OrderCreated{{
		ID:         cmd.ID,
		CustomerID: cmd.CustomerID,
		Amount:     cmd.Amount,
		CreatedAt:  time.Now(),
	}}, nil
}

func handleChargePayment(ctx context.Context, cmd ChargePayment) ([]PaymentCharged, error) {
	log.Printf("   💳 Charging $%d...", cmd.Amount)
	time.Sleep(100 * time.Millisecond)
	return []PaymentCharged{{
		OrderID:   cmd.OrderID,
		Amount:    cmd.Amount,
		ChargedAt: time.Now(),
	}}, nil
}

func handleReserveInventory(ctx context.Context, cmd ReserveInventory) ([]InventoryReserved, error) {
	log.Printf("   📦 Reserving inventory for %s...", cmd.SKU)
	time.Sleep(100 * time.Millisecond)
	return []InventoryReserved{{
		OrderID:    cmd.OrderID,
		ReservedAt: time.Now(),
	}}, nil
}

func handleShipOrder(ctx context.Context, cmd ShipOrder) ([]OrderShipped, error) {
	log.Printf("   🚚 Shipping to %s...", cmd.Address)
	time.Sleep(100 * time.Millisecond)
	return []OrderShipped{{
		OrderID:    cmd.OrderID,
		TrackingID: "TRACK-" + cmd.OrderID,
		ShippedAt:  time.Now(),
	}}, nil
}

// ============================================================================
// Event Handlers (Events → Side Effects)
// ============================================================================

func handleOrderCreatedEmail(ctx context.Context, evt OrderCreated) error {
	log.Printf("📧 Side Effect: Sending order confirmation email to %s", evt.CustomerID)
	time.Sleep(50 * time.Millisecond)
	return nil
}

func handleOrderCreatedAnalytics(ctx context.Context, evt OrderCreated) error {
	log.Printf("📊 Side Effect: Tracking order_created event (amount: $%d)", evt.Amount)
	return nil
}

func handlePaymentChargedAnalytics(ctx context.Context, evt PaymentCharged) error {
	log.Printf("📊 Side Effect: Tracking payment_charged event (amount: $%d)", evt.Amount)
	return nil
}

func handleOrderShippedEmail(ctx context.Context, evt OrderShipped) error {
	log.Printf("📧 Side Effect: Sending shipping notification (tracking: %s)", evt.TrackingID)
	time.Sleep(50 * time.Millisecond)
	return nil
}

// ============================================================================
// Saga Coordinator (Events → Commands, Workflow Logic)
// ============================================================================

type OrderSagaCoordinator struct {
	marshaler cqrs.CommandMarshaler
}

func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	subject, _ := msg.Attributes.Subject()
	corrID, _ := msg.Attributes.CorrelationID()

	switch subject {
	case "OrderCreated":
		var evt OrderCreated
		s.marshaler.Unmarshal(msg.Data, &evt)

		log.Printf("🔄 Saga: OrderCreated → triggering ChargePayment + ReserveInventory")

		// ✅ One event triggers MULTIPLE commands (multistage acking!)
		return createCommands(s.marshaler, corrID,
			ChargePayment{
				OrderID:    evt.ID,
				CustomerID: evt.CustomerID,
				Amount:     evt.Amount,
			},
			ReserveInventory{
				OrderID: evt.ID,
				SKU:     "SKU-12345",
			},
		), nil

	case "PaymentCharged":
		var evt PaymentCharged
		s.marshaler.Unmarshal(msg.Data, &evt)

		log.Printf("🔄 Saga: PaymentCharged → waiting for InventoryReserved...")
		return nil, nil

	case "InventoryReserved":
		var evt InventoryReserved
		s.marshaler.Unmarshal(msg.Data, &evt)

		log.Printf("🔄 Saga: InventoryReserved → triggering ShipOrder")

		return createCommands(s.marshaler, corrID,
			ShipOrder{
				OrderID: evt.OrderID,
				Address: "123 Main St",
			},
		), nil

	case "OrderShipped":
		log.Printf("✅ Saga: OrderShipped → saga complete!")
		return nil, nil // Terminal
	}

	return nil, nil
}

// ============================================================================
// Main
// ============================================================================

func main() {
	ctx := context.Background()
	marshaler := cqrs.NewJSONCommandMarshaler(
		cqrs.WithType(),
	)

	log.Println(strings.Repeat("=", 70))
	log.Println("CQRS Package Example - Order Processing Saga")
	log.Println(strings.Repeat("=", 70))
	log.Println()

	// ========================================================================
	// Command Handlers
	// ========================================================================

	createOrderHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			log.Printf("📝 Command: CreateOrder")
			return handleCreateOrder(ctx, cmd)
		},
		cqrs.Match(cqrs.MatchSubject("CreateOrder"), cqrs.MatchType("command")),
		marshaler,
	)

	chargePaymentHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, cmd ChargePayment) ([]PaymentCharged, error) {
			log.Printf("📝 Command: ChargePayment")
			return handleChargePayment(ctx, cmd)
		},
		cqrs.Match(cqrs.MatchSubject("ChargePayment"), cqrs.MatchType("command")),
		marshaler,
	)

	reserveInventoryHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, cmd ReserveInventory) ([]InventoryReserved, error) {
			log.Printf("📝 Command: ReserveInventory")
			return handleReserveInventory(ctx, cmd)
		},
		cqrs.Match(cqrs.MatchSubject("ReserveInventory"), cqrs.MatchType("command")),
		marshaler,
	)

	shipOrderHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, cmd ShipOrder) ([]OrderShipped, error) {
			log.Printf("📝 Command: ShipOrder")
			return handleShipOrder(ctx, cmd)
		},
		cqrs.Match(cqrs.MatchSubject("ShipOrder"), cqrs.MatchType("command")),
		marshaler,
	)

	commandRouter := cqrs.NewRouter(cqrs.RouterConfig{
		Concurrency: 10,
		Recover:     true,
	})
	commandRouter.AddHandler(createOrderHandler)
	commandRouter.AddHandler(chargePaymentHandler)
	commandRouter.AddHandler(reserveInventoryHandler)
	commandRouter.AddHandler(shipOrderHandler)

	// ========================================================================
	// Event Handlers (Side Effects)
	// ========================================================================

	sideEffectsRouter := cqrs.NewRouter(cqrs.RouterConfig{
		Concurrency: 20,
		Recover:     true,
	})
	sideEffectsRouter.AddHandler(cqrs.NewEventHandler(handleOrderCreatedEmail, cqrs.Match(cqrs.MatchSubject("OrderCreated"), cqrs.MatchType("event")), marshaler))
	sideEffectsRouter.AddHandler(cqrs.NewEventHandler(handleOrderCreatedAnalytics, cqrs.Match(cqrs.MatchSubject("OrderCreated"), cqrs.MatchType("event")), marshaler))
	sideEffectsRouter.AddHandler(cqrs.NewEventHandler(handlePaymentChargedAnalytics, cqrs.Match(cqrs.MatchSubject("PaymentCharged"), cqrs.MatchType("event")), marshaler))
	sideEffectsRouter.AddHandler(cqrs.NewEventHandler(handleOrderShippedEmail, cqrs.Match(cqrs.MatchSubject("OrderShipped"), cqrs.MatchType("event")), marshaler))

	// ========================================================================
	// Saga Coordinator
	// ========================================================================

	sagaCoordinator := &OrderSagaCoordinator{marshaler: marshaler}
	sagaHandler := cqrs.NewHandler(
		sagaCoordinator.OnEvent,
		func(prop message.Attributes) bool {
			msgType, _ := prop["type"].(string)
			return msgType == "event" // Reacts to ALL events
		},
	)

	sagaRouter := cqrs.NewRouter(cqrs.RouterConfig{Recover: true})
	sagaRouter.AddHandler(sagaHandler)

	// ========================================================================
	// Wire Together
	// ========================================================================

	initialCommands := make(chan *message.Message, 10)
	sagaCommands := make(chan *message.Message, 100)

	// Merge initial + saga-triggered commands
	allCommands := channel.Merge(initialCommands, sagaCommands)

	// Commands → Events
	events := commandRouter.Start(ctx, allCommands)

	// Broadcast events to both side effects AND saga coordinator
	eventChan1 := make(chan *message.Message, 100)
	eventChan2 := make(chan *message.Message, 100)

	// Fan-out events
	go func() {
		for evt := range events {
			eventChan1 <- evt
			eventChan2 <- evt
		}
		close(eventChan1)
		close(eventChan2)
	}()

	// Side effects processor
	sideEffectsOut := sideEffectsRouter.Start(ctx, eventChan1)
	go func() {
		for range sideEffectsOut {
		} // Drain
	}()

	// Saga coordinator (workflow logic)
	sagaOut := sagaRouter.Start(ctx, eventChan2)

	// Feedback loop: route saga commands back
	go func() {
		for cmd := range sagaOut {
			select {
			case sagaCommands <- cmd:
			case <-ctx.Done():
				return
			}
		}
	}()

	// ========================================================================
	// Demo
	// ========================================================================

	log.Println("Flow:")
	log.Println("  CreateOrder (command)")
	log.Println("    → OrderCreated (event)")
	log.Println("      → Email + Analytics (side effects)")
	log.Println("      → ChargePayment + ReserveInventory (saga commands)")
	log.Println("    → PaymentCharged + InventoryReserved (events)")
	log.Println("      → ShipOrder (saga command)")
	log.Println("    → OrderShipped (event)")
	log.Println("      → Email (side effect)")
	log.Println()
	log.Println("🚀 Sending CreateOrder command...")
	log.Println()

	initialCommands <- createCommand(
		marshaler,
		CreateOrder{
			ID:         "order-789",
			CustomerID: "customer-456",
			Amount:     350,
		},
		message.Attributes{
			message.AttrCorrelationID: "corr-123",
		},
	)

	time.Sleep(2 * time.Second)

	close(initialCommands)
	time.Sleep(500 * time.Millisecond)

	log.Println()
	log.Println(strings.Repeat("=", 70))
	log.Println("Demo Complete!")
	log.Println()
	log.Println("Key Benefits of cqrs Package:")
	log.Println("  ✅ Type-safe command and event handlers")
	log.Println("  ✅ Clean separation: side effects vs workflow logic")
	log.Println("  ✅ Pluggable marshalers (JSON, Protobuf, etc.)")
	log.Println("  ✅ Built on gopipe's channel-based architecture")
	log.Println("  ✅ Easy to test: pure functions")
	log.Println(strings.Repeat("=", 70))
}
