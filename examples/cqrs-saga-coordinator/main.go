package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// ============================================================================
// Handler and Router (local POC types)
// ============================================================================

// Handler processes messages matching specific properties (local POC type)
type Handler interface {
	Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error)
	Match(prop message.Attributes) bool
}

type handler struct {
	handle func(ctx context.Context, msg *message.Message) ([]*message.Message, error)
	match  func(prop message.Attributes) bool
}

func (h *handler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	return h.handle(ctx, msg)
}

func (h *handler) Match(prop message.Attributes) bool {
	return h.match(prop)
}

// NewHandler creates a new Handler (local POC function)
func NewHandler(
	handle func(ctx context.Context, msg *message.Message) ([]*message.Message, error),
	match func(prop message.Attributes) bool,
) Handler {
	return &handler{
		handle: handle,
		match:  match,
	}
}

// RouterConfig configures message routing (local POC type)
type RouterConfig struct {
	Concurrency int
	Recover     bool
}

// Router dispatches messages to handlers (local POC type)
type Router struct {
	handlers []Handler
	config   RouterConfig
}

// NewRouter creates a new Router (local POC function)
func NewRouter(config RouterConfig, handlers ...Handler) *Router {
	return &Router{
		handlers: handlers,
		config:   config,
	}
}

// Start begins processing messages
func (r *Router) Start(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	handle := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		for _, h := range r.handlers {
			if h.Match(msg.Attributes) {
				return h.Handle(ctx, msg)
			}
		}
		err := fmt.Errorf("no handler matched")
		msg.Nack(err)
		return nil, err
	}

	opts := []gopipe.Option[*message.Message, *message.Message]{
		gopipe.WithConcurrency[*message.Message, *message.Message](r.config.Concurrency),
	}
	if r.config.Recover {
		opts = append(opts, gopipe.WithRecover[*message.Message, *message.Message]())
	}

	return gopipe.NewProcessPipe(handle, opts...).Start(ctx, msgs)
}

// ============================================================================
// Marshaler
// ============================================================================

type Marshaler interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
	Name(v any) string
}

type JSONMarshaler struct{}

func (m JSONMarshaler) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (m JSONMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (m JSONMarshaler) Name(v any) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
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

func NewCommandHandler[Cmd, Evt any](
	cmdName string,
	marshal Marshaler,
	handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
) Handler {
	return NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			var cmd Cmd
			if err := json.Unmarshal(msg.Data, &cmd); err != nil {
				msg.Nack(err)
				return nil, err
			}

			log.Printf("📝 Command: %s", cmdName)

			events, err := handle(ctx, cmd)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}

			var outMsgs []*message.Message
			for _, evt := range events {
				evtName := marshal.Name(evt)
				log.Printf("   ✅ → Event: %s", evtName)

				payload, _ := json.Marshal(evt)
				props := message.Attributes{
					message.AttrSubject: evtName,
					"type":              "event",
				}
				if corrID, ok := msg.Attributes.CorrelationID(); ok {
					props[message.AttrCorrelationID] = corrID
				}

				outMsgs = append(outMsgs, message.New(payload, props))
			}

			msg.Ack()
			return outMsgs, nil
		},
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			msgType, _ := prop["type"].(string)
			return subject == cmdName && msgType == "command"
		},
	)
}

// ============================================================================
// Pure Event Handlers (Events → Side Effects ONLY, no commands!)
// ============================================================================

type EmailHandler struct{}

func (h *EmailHandler) HandleOrderCreated(ctx context.Context, evt OrderCreated) error {
	log.Printf("📧 Side Effect: Sending order confirmation email to %s", evt.CustomerID)
	time.Sleep(50 * time.Millisecond)  // Simulate email send
	return nil
}

func (h *EmailHandler) HandleOrderShipped(ctx context.Context, evt OrderShipped) error {
	log.Printf("📧 Side Effect: Sending shipping notification (tracking: %s)", evt.TrackingID)
	time.Sleep(50 * time.Millisecond)
	return nil
}

type AnalyticsHandler struct{}

func (h *AnalyticsHandler) HandleOrderCreated(ctx context.Context, evt OrderCreated) error {
	log.Printf("📊 Side Effect: Tracking order_created event (amount: $%d)", evt.Amount)
	return nil
}

func (h *AnalyticsHandler) HandlePaymentCharged(ctx context.Context, evt PaymentCharged) error {
	log.Printf("📊 Side Effect: Tracking payment_charged event (amount: $%d)", evt.Amount)
	return nil
}

// Helper to create event handler
func NewEventHandler[Evt any](
	evtName string,
	handle func(ctx context.Context, evt Evt) error,
) Handler {
	return NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			var evt Evt
			if err := json.Unmarshal(msg.Data, &evt); err != nil {
				msg.Nack(err)
				return nil, err
			}

			if err := handle(ctx, evt); err != nil {
				msg.Nack(err)
				return nil, err
			}

			msg.Ack()
			return nil, nil  // No output messages!
		},
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			msgType, _ := prop["type"].(string)
			return subject == evtName && msgType == "event"
		},
	)
}

// ============================================================================
// Saga Coordinator (Events → Commands, workflow logic)
// ============================================================================

type OrderSagaCoordinator struct {
	marshaler Marshaler
}

// OnEvent handles events and returns commands to trigger next saga steps
// ✅ This is where workflow logic lives (decoupled from event handlers!)
func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	subject, _ := msg.Attributes.Subject()

	switch subject {
	case "OrderCreated":
		var evt OrderCreated
		json.Unmarshal(msg.Data, &evt)

		log.Printf("🔄 Saga: OrderCreated → triggering ChargePayment + ReserveInventory")

		// ✅ One event triggers MULTIPLE commands (multistage acking!)
		return s.createCommands(msg,
			ChargePayment{
				OrderID:    evt.ID,
				CustomerID: evt.CustomerID,
				Amount:     evt.Amount,
			},
			ReserveInventory{
				OrderID: evt.ID,
				SKU:     "SKU-12345",
			},
		)

	case "PaymentCharged":
		var evt PaymentCharged
		json.Unmarshal(msg.Data, &evt)

		// Wait for inventory reservation too...
		// (In real implementation, would check if both completed)
		log.Printf("🔄 Saga: PaymentCharged → waiting for InventoryReserved...")
		return nil, nil

	case "InventoryReserved":
		var evt InventoryReserved
		json.Unmarshal(msg.Data, &evt)

		log.Printf("🔄 Saga: InventoryReserved → triggering ShipOrder")

		return s.createCommands(msg,
			ShipOrder{
				OrderID: evt.OrderID,
				Address: "123 Main St",
			},
		)

	case "OrderShipped":
		log.Printf("✅ Saga: OrderShipped → saga complete!")
		return nil, nil  // Terminal
	}

	return nil, nil
}

func (s *OrderSagaCoordinator) createCommands(originalMsg *message.Message, cmds ...any) ([]*message.Message, error) {
	var msgs []*message.Message

	for _, cmd := range cmds {
		payload, _ := s.marshaler.Marshal(cmd)
		name := s.marshaler.Name(cmd)

		props := message.Attributes{
			message.AttrSubject: name,
			"type":              "command",
		}

		// Propagate correlation ID
		if corrID, ok := originalMsg.Attributes.CorrelationID(); ok {
			props[message.AttrCorrelationID] = corrID
		}

		msgs = append(msgs, message.New(payload, props))
	}

	return msgs, nil
}

// ============================================================================
// Helper to create command message
// ============================================================================

func createCommand(marshal Marshaler, cmd any, corrID string) *message.Message {
	payload, _ := marshal.Marshal(cmd)
	return message.New(payload, message.Attributes{
		message.AttrSubject:       marshal.Name(cmd),
		message.AttrCorrelationID: corrID,
		message.AttrTime:     time.Now(),
		"type":                    "command",
	})
}

// ============================================================================
// Main
// ============================================================================

func main() {
	ctx := context.Background()
	marshaler := JSONMarshaler{}

	log.Println(strings.Repeat("=", 70))
	log.Println("CQRS with Saga Coordinator Pattern")
	log.Println("✅ Event handlers do NOT return commands (decoupled!)")
	log.Println("✅ Saga coordinator manages workflow logic separately")
	log.Println(strings.Repeat("=", 70))
	log.Println()

	// ========================================================================
	// Command Handlers
	// ========================================================================

	createOrderHandler := NewCommandHandler(
		"CreateOrder",
		marshaler,
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			log.Printf("   💾 Saving order to database...")
			return []OrderCreated{{
				ID:         cmd.ID,
				CustomerID: cmd.CustomerID,
				Amount:     cmd.Amount,
				CreatedAt:  time.Now(),
			}}, nil
		},
	)

	chargePaymentHandler := NewCommandHandler(
		"ChargePayment",
		marshaler,
		func(ctx context.Context, cmd ChargePayment) ([]PaymentCharged, error) {
			log.Printf("   💳 Charging $%d...", cmd.Amount)
			time.Sleep(100 * time.Millisecond)
			return []PaymentCharged{{
				OrderID:   cmd.OrderID,
				Amount:    cmd.Amount,
				ChargedAt: time.Now(),
			}}, nil
		},
	)

	reserveInventoryHandler := NewCommandHandler(
		"ReserveInventory",
		marshaler,
		func(ctx context.Context, cmd ReserveInventory) ([]InventoryReserved, error) {
			log.Printf("   📦 Reserving inventory for %s...", cmd.SKU)
			time.Sleep(100 * time.Millisecond)
			return []InventoryReserved{{
				OrderID:    cmd.OrderID,
				ReservedAt: time.Now(),
			}}, nil
		},
	)

	shipOrderHandler := NewCommandHandler(
		"ShipOrder",
		marshaler,
		func(ctx context.Context, cmd ShipOrder) ([]OrderShipped, error) {
			log.Printf("   🚚 Shipping to %s...", cmd.Address)
			time.Sleep(100 * time.Millisecond)
			return []OrderShipped{{
				OrderID:    cmd.OrderID,
				TrackingID: "TRACK-" + cmd.OrderID,
				ShippedAt:  time.Now(),
			}}, nil
		},
	)

	commandProcessor := NewRouter(
		RouterConfig{
			Concurrency: 10,
			Recover:     true,
		},
		createOrderHandler,
		chargePaymentHandler,
		reserveInventoryHandler,
		shipOrderHandler,
	)

	// ========================================================================
	// Pure Event Handlers (Side effects only!)
	// ========================================================================

	emailHandler := &EmailHandler{}
	analyticsHandler := &AnalyticsHandler{}

	sideEffectsRouter := NewRouter(
		RouterConfig{
			Concurrency: 20,
			Recover:     true,
		},
		NewEventHandler("OrderCreated", emailHandler.HandleOrderCreated),
		NewEventHandler("OrderCreated", analyticsHandler.HandleOrderCreated),
		NewEventHandler("PaymentCharged", analyticsHandler.HandlePaymentCharged),
		NewEventHandler("OrderShipped", emailHandler.HandleOrderShipped),
	)

	// ========================================================================
	// Saga Coordinator (Workflow logic)
	// ========================================================================

	sagaCoordinator := &OrderSagaCoordinator{marshaler: marshaler}
	sagaHandler := NewHandler(
		sagaCoordinator.OnEvent,
		func(prop message.Attributes) bool {
			msgType, _ := prop["type"].(string)
			return msgType == "event"  // Reacts to ALL events
		},
	)

	sagaRouter := NewRouter(
		RouterConfig{Recover: true},
		sagaHandler,
	)

	// ========================================================================
	// Wire Together
	// ========================================================================

	initialCommands := make(chan *message.Message, 10)
	sagaCommands := make(chan *message.Message, 100)

	// Merge initial + saga-triggered commands
	allCommands := channel.Merge(initialCommands, sagaCommands)

	// Commands → Events
	events := commandProcessor.Start(ctx, allCommands)

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

	// Side effects processor (emails, analytics, etc.)
	sideEffectsOut := sideEffectsRouter.Start(ctx, eventChan1)
	go func() {
		for range sideEffectsOut {
		}  // Drain
	}()

	// Saga coordinator (workflow logic)
	sagaOut := sagaRouter.Start(ctx, eventChan2)

	// Route saga commands back
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
	log.Println("  CreateOrder")
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

	initialCommands <- createCommand(marshaler, CreateOrder{
		ID:         "order-789",
		CustomerID: "customer-456",
		Amount:     350,
	}, "corr-123")

	time.Sleep(2 * time.Second)

	close(initialCommands)
	time.Sleep(500 * time.Millisecond)

	log.Println()
	log.Println(strings.Repeat("=", 70))
	log.Println("Demo Complete!")
	log.Println()
	log.Println("Key Benefits:")
	log.Println("  ✅ Event handlers are DECOUPLED from commands")
	log.Println("  ✅ Workflow logic is in Saga Coordinator (not event handlers)")
	log.Println("  ✅ Easy to test: side effects vs saga logic separately")
	log.Println("  ✅ Multistage acking: one event → multiple commands")
	log.Println("  ✅ Clean separation: side effects vs workflow")
	log.Println(strings.Repeat("=", 70))
}
