package main

import (
	"context"
	"encoding/json"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// ============================================================================
// CQRS Implementation (Revised Design)
// - Command handlers RETURN events (not send to bus)
// - Event handlers can RETURN commands (saga pattern)
// - Processors return output channels (gopipe pattern)
// ============================================================================

// Marshaler serializes commands/events to messages
type Marshaler interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
	Name(v any) string
}

// JSONMarshaler implements Marshaler using JSON
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
// CommandProcessor: Commands → Events
// ============================================================================

type CommandProcessor struct {
	router  *message.Router
	marshal Marshaler
}

func NewCommandProcessor(config message.RouterConfig, marshal Marshaler, handlers ...message.Handler) *CommandProcessor {
	return &CommandProcessor{
		router:  message.NewRouter(config, handlers...),
		marshal: marshal,
	}
}

func (p *CommandProcessor) Start(ctx context.Context, commands <-chan *message.Message) <-chan *message.Message {
	return p.router.Start(ctx, commands)
}

// NewCommandHandler creates a command handler that returns events
func NewCommandHandler[Cmd, Evt any](
	cmdName string,
	marshal Marshaler,
	handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
) message.Handler {
	// We need a custom handler instead of JSONHandler because we need to set
	// the subject based on the OUTPUT event type, not the input command
	return message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Unmarshal command
			var cmd Cmd
			if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
				msg.Nack(err)
				return nil, err
			}

			log.Printf("📝 Processing command: %s", cmdName)

			// Handle command → produce events
			events, err := handle(ctx, cmd)
			if err != nil {
				log.Printf("❌ Command failed: %v", err)
				msg.Nack(err)
				return nil, err
			}

			// Marshal events
			var outMsgs []*message.Message
			for _, evt := range events {
				evtName := marshal.Name(evt)
				log.Printf("✅ Command produced event: %s", evtName)

				payload, err := json.Marshal(evt)
				if err != nil {
					msg.Nack(err)
					return nil, err
				}

				// Set proper subject for event routing
				props := message.Properties{
					message.PropSubject: evtName,
					"type":              "event",
				}
				if corrID, ok := msg.Properties.CorrelationID(); ok {
					props[message.PropCorrelationID] = corrID
				}

				outMsgs = append(outMsgs, message.New(payload, props))
			}

			msg.Ack()
			return outMsgs, nil
		},
		func(prop message.Properties) bool {
			subject, _ := prop.Subject()
			msgType, _ := prop["type"].(string)
			return subject == cmdName && msgType == "command"
		},
	)
}

// ============================================================================
// EventProcessor: Events → Commands (for sagas)
// ============================================================================

type EventProcessor struct {
	router  *message.Router
	marshal Marshaler
}

func NewEventProcessor(config message.RouterConfig, marshal Marshaler, handlers ...message.Handler) *EventProcessor {
	return &EventProcessor{
		router:  message.NewRouter(config, handlers...),
		marshal: marshal,
	}
}

func (p *EventProcessor) Start(ctx context.Context, events <-chan *message.Message) <-chan *message.Message {
	return p.router.Start(ctx, events)
}

// NewEventHandler creates an event handler that can return commands (saga pattern)
func NewEventHandler[Evt, Out any](
	evtName string,
	marshal Marshaler,
	handle func(ctx context.Context, evt Evt) ([]Out, error),
) message.Handler {
	return message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Unmarshal event
			var evt Evt
			if err := json.Unmarshal(msg.Payload, &evt); err != nil {
				msg.Nack(err)
				return nil, err
			}

			log.Printf("📨 Processing event: %s", evtName)

			// Handle event → produce commands (or nothing)
			outputs, err := handle(ctx, evt)
			if err != nil {
				log.Printf("❌ Event handler failed: %v", err)
				msg.Nack(err)
				return nil, err
			}

			// Marshal outputs (commands)
			var outMsgs []*message.Message
			for _, out := range outputs {
				outName := marshal.Name(out)
				log.Printf("🔄 Event handler produced: %s", outName)

				payload, err := json.Marshal(out)
				if err != nil {
					msg.Nack(err)
					return nil, err
				}

				// Set proper subject for command routing
				props := message.Properties{
					message.PropSubject: outName,
					"type":              "command",
				}
				if corrID, ok := msg.Properties.CorrelationID(); ok {
					props[message.PropCorrelationID] = corrID
				}

				outMsgs = append(outMsgs, message.New(payload, props))
			}

			msg.Ack()
			return outMsgs, nil
		},
		func(prop message.Properties) bool {
			subject, _ := prop.Subject()
			msgType, _ := prop["type"].(string)
			return subject == evtName && msgType == "event"
		},
	)
}

// ============================================================================
// Domain: Order Processing with Saga
// ============================================================================

// Commands (imperative)
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

type ShipOrder struct {
	OrderID string `json:"order_id"`
	Address string `json:"address"`
}

// Events (past tense)
type OrderCreated struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Amount     int       `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
}

type PaymentCharged struct {
	OrderID    string    `json:"order_id"`
	CustomerID string    `json:"customer_id"`
	Amount     int       `json:"amount"`
	ChargedAt  time.Time `json:"charged_at"`
}

type OrderShipped struct {
	OrderID    string    `json:"order_id"`
	ShippedAt  time.Time `json:"shipped_at"`
	TrackingID string    `json:"tracking_id"`
}

// Helper to create command message
func createCommand(marshal Marshaler, cmd any, corrID string) *message.Message {
	payload, _ := marshal.Marshal(cmd)
	return message.New(payload, message.Properties{
		message.PropSubject:       marshal.Name(cmd),
		message.PropCorrelationID: corrID,
		message.PropCreatedAt:     time.Now(),
		"type":                    "command",
	})
}

// ============================================================================
// Main: Demonstrates Saga Pattern
// ============================================================================

func main() {
	ctx := context.Background()
	marshaler := JSONMarshaler{}

	log.Println(strings.Repeat("=", 70))
	log.Println("CQRS Example: Order Processing with Saga Pattern")
	log.Println(strings.Repeat("=", 70))
	log.Println()

	// ========================================================================
	// Command Handlers (return events, not send to bus!)
	// ========================================================================

	createOrderHandler := NewCommandHandler(
		"CreateOrder",
		marshaler,
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			// Business logic
			log.Printf("   💾 Saving order to database...")

			// Return event (not send to bus!)
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
			// Business logic
			log.Printf("   💳 Charging payment...")
			time.Sleep(100 * time.Millisecond)

			return []PaymentCharged{{
				OrderID:    cmd.OrderID,
				CustomerID: cmd.CustomerID,
				Amount:     cmd.Amount,
				ChargedAt:  time.Now(),
			}}, nil
		},
	)

	shipOrderHandler := NewCommandHandler(
		"ShipOrder",
		marshaler,
		func(ctx context.Context, cmd ShipOrder) ([]OrderShipped, error) {
			// Business logic
			log.Printf("   📦 Shipping order...")
			time.Sleep(100 * time.Millisecond)

			return []OrderShipped{{
				OrderID:    cmd.OrderID,
				ShippedAt:  time.Now(),
				TrackingID: "TRACK-" + cmd.OrderID,
			}}, nil
		},
	)

	// ========================================================================
	// Event Handlers (can return commands for saga pattern!)
	// ========================================================================

	// Saga Step 1: OrderCreated → ChargePayment command
	orderCreatedSagaHandler := NewEventHandler(
		"OrderCreated",
		marshaler,
		func(ctx context.Context, evt OrderCreated) ([]ChargePayment, error) {
			log.Printf("   🔄 Saga: OrderCreated → trigger ChargePayment")

			// Return command (saga pattern!)
			return []ChargePayment{{
				OrderID:    evt.ID,
				CustomerID: evt.CustomerID,
				Amount:     evt.Amount,
			}}, nil
		},
	)

	// Saga Step 2: PaymentCharged → ShipOrder command
	paymentChargedSagaHandler := NewEventHandler(
		"PaymentCharged",
		marshaler,
		func(ctx context.Context, evt PaymentCharged) ([]ShipOrder, error) {
			log.Printf("   🔄 Saga: PaymentCharged → trigger ShipOrder")

			// Return command (saga pattern!)
			return []ShipOrder{{
				OrderID: evt.OrderID,
				Address: "123 Main St",
			}}, nil
		},
	)

	// Terminal handler: Just send email (no commands)
	type NoOutput struct{}

	orderShippedEmailHandler := NewEventHandler(
		"OrderShipped",
		marshaler,
		func(ctx context.Context, evt OrderShipped) ([]NoOutput, error) {
			log.Printf("   📧 Sending shipping confirmation email")
			log.Printf("   ✉️  Email sent! Tracking: %s", evt.TrackingID)
			return nil, nil // Terminal
		},
	)

	// ========================================================================
	// Create Processors
	// ========================================================================

	commandProcessor := NewCommandProcessor(
		message.RouterConfig{
			Concurrency: 5,
			Recover:     true,
		},
		marshaler,
		createOrderHandler,
		chargePaymentHandler,
		shipOrderHandler,
	)

	eventProcessor := NewEventProcessor(
		message.RouterConfig{
			Concurrency: 10,
			Recover:     true,
		},
		marshaler,
		orderCreatedSagaHandler,
		paymentChargedSagaHandler,
		orderShippedEmailHandler,
	)

	// ========================================================================
	// Wire Together: Commands → Events → Commands (Saga feedback loop)
	// ========================================================================

	// Initial command channel
	initialCommands := make(chan *message.Message, 10)

	// Create feedback channel for saga-triggered commands
	sagaCommands := make(chan *message.Message, 100)

	// Merge initial commands + saga-triggered commands (feedback loop!)
	allCommands := channel.Merge(initialCommands, sagaCommands)

	// Commands → Events
	events := commandProcessor.Start(ctx, allCommands)

	// Events → Commands (saga) or terminal
	sagaOutputs := eventProcessor.Start(ctx, events)

	// Route saga outputs back to commands channel
	go func() {
		for cmd := range sagaOutputs {
			select {
			case sagaCommands <- cmd:
			case <-ctx.Done():
				return
			}
		}
	}()

	// ========================================================================
	// Demo: Send CreateOrder and watch the saga unfold
	// ========================================================================

	log.Println("Flow: CreateOrder → OrderCreated → ChargePayment → PaymentCharged → ShipOrder → OrderShipped")
	log.Println()
	log.Println("🚀 Sending CreateOrder command...")
	log.Println()

	// Send initial command
	initialCommands <- createCommand(marshaler, CreateOrder{
		ID:         "order-789",
		CustomerID: "customer-456",
		Amount:     350,
	}, "corr-123")

	// Wait for saga to complete
	time.Sleep(2 * time.Second)

	close(initialCommands)
	time.Sleep(500 * time.Millisecond)

	log.Println()
	log.Println(strings.Repeat("=", 70))
	log.Println("Saga Complete!")
	log.Println()
	log.Println("What happened:")
	log.Println("  1. CreateOrder command → OrderCreated event")
	log.Println("  2. OrderCreated event → ChargePayment command (saga)")
	log.Println("  3. ChargePayment command → PaymentCharged event")
	log.Println("  4. PaymentCharged event → ShipOrder command (saga)")
	log.Println("  5. ShipOrder command → OrderShipped event")
	log.Println("  6. OrderShipped event → Send email (terminal)")
	log.Println()
	log.Println("Key improvements:")
	log.Println("  ✅ Command handlers RETURN events (not send to bus)")
	log.Println("  ✅ Event handlers RETURN commands (saga pattern)")
	log.Println("  ✅ Processors return channels (gopipe pattern)")
	log.Println("  ✅ Natural feedback loop via channel.Merge")
	log.Println("  ✅ Handlers are pure functions (easy to test)")
	log.Println(strings.Repeat("=", 70))
}
