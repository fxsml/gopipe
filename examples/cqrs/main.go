package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/fxsml/gopipe/message"
)

// ============================================================================
// CQRS Implementation (Proof of Concept)
// This demonstrates how the proposed cqrs package would work
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

// CommandBus sends commands to handlers
type CommandBus struct {
	marshaler Marshaler
	out       chan<- *message.Message
}

func NewCommandBus(marshaler Marshaler, out chan<- *message.Message) *CommandBus {
	return &CommandBus{
		marshaler: marshaler,
		out:       out,
	}
}

func (b *CommandBus) Send(ctx context.Context, cmd any) error {
	payload, err := b.marshaler.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}

	name := b.marshaler.Name(cmd)
	msg := message.New(payload, message.Properties{
		message.PropSubject:   name,
		message.PropCreatedAt: time.Now(),
		"type":                "command",
	})

	select {
	case b.out <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// EventBus publishes events to subscribers
type EventBus struct {
	marshaler Marshaler
	out       chan<- *message.Message
}

func NewEventBus(marshaler Marshaler, out chan<- *message.Message) *EventBus {
	return &EventBus{
		marshaler: marshaler,
		out:       out,
	}
}

func (b *EventBus) Publish(ctx context.Context, evt any) error {
	payload, err := b.marshaler.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	name := b.marshaler.Name(evt)
	msg := message.New(payload, message.Properties{
		message.PropSubject:   name,
		message.PropCreatedAt: time.Now(),
		"type":                "event",
	})

	select {
	case b.out <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewCommandHandler creates a typed command handler
func NewCommandHandler[T any](
	name string,
	handle func(ctx context.Context, cmd T) error,
) *message.Handler {
	return message.NewHandler(
		func(ctx context.Context, cmd T) ([]T, error) {
			// Commands don't produce output messages directly
			// They publish events via EventBus instead
			err := handle(ctx, cmd)
			return nil, err
		},
		func(prop map[string]any) bool {
			subject, _ := message.SubjectProps(prop)
			msgType, _ := prop["type"].(string)
			return subject == name && msgType == "command"
		},
		func(prop map[string]any) map[string]any {
			// Commands don't transform properties
			return nil
		},
	)
}

// NewEventHandler creates a typed event handler
func NewEventHandler[T any](
	name string,
	handle func(ctx context.Context, evt T) error,
) *message.Handler {
	return message.NewHandler(
		func(ctx context.Context, evt T) ([]T, error) {
			err := handle(ctx, evt)
			return nil, err
		},
		func(prop map[string]any) bool {
			subject, _ := message.SubjectProps(prop)
			msgType, _ := prop["type"].(string)
			return subject == name && msgType == "event"
		},
		func(prop map[string]any) map[string]any {
			return nil
		},
	)
}

// ============================================================================
// Domain: Order Processing
// ============================================================================

// Commands (imperative - requests for actions)
type CreateOrder struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
}

type CancelOrder struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Events (past tense - facts about what happened)
type OrderCreated struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Amount     int       `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
}

type OrderCancelled struct {
	ID          string    `json:"id"`
	Reason      string    `json:"reason"`
	CancelledAt time.Time `json:"cancelled_at"`
}

type EmailSent struct {
	CustomerID string `json:"customer_id"`
	OrderID    string `json:"order_id"`
	Type       string `json:"type"`
}

// ============================================================================
// Main
// ============================================================================

func main() {
	ctx := context.Background()
	marshaler := JSONMarshaler{}

	// Create channels for commands and events
	commandChan := make(chan *message.Message, 10)
	eventChan := make(chan *message.Message, 10)

	// Create buses
	commandBus := NewCommandBus(marshaler, commandChan)
	eventBus := NewEventBus(marshaler, eventChan)

	// ========================================================================
	// Command Handlers (Process commands, publish events)
	// ========================================================================

	createOrderHandler := NewCommandHandler(
		"CreateOrder",
		func(ctx context.Context, cmd CreateOrder) error {
			log.Printf("📝 Handling command: CreateOrder{ID: %s, Amount: %d}", cmd.ID, cmd.Amount)

			// Validate
			if cmd.Amount <= 0 {
				return fmt.Errorf("invalid amount: %d", cmd.Amount)
			}

			// Business logic (would save to database in real app)
			log.Printf("✅ Order created: %s", cmd.ID)

			// Publish event
			return eventBus.Publish(ctx, OrderCreated{
				ID:         cmd.ID,
				CustomerID: cmd.CustomerID,
				Amount:     cmd.Amount,
				CreatedAt:  time.Now(),
			})
		},
	)

	cancelOrderHandler := NewCommandHandler(
		"CancelOrder",
		func(ctx context.Context, cmd CancelOrder) error {
			log.Printf("📝 Handling command: CancelOrder{ID: %s, Reason: %s}", cmd.ID, cmd.Reason)

			// Business logic
			log.Printf("❌ Order cancelled: %s", cmd.ID)

			// Publish event
			return eventBus.Publish(ctx, OrderCancelled{
				ID:          cmd.ID,
				Reason:      cmd.Reason,
				CancelledAt: time.Now(),
			})
		},
	)

	// ========================================================================
	// Event Handlers (React to events)
	// Multiple handlers can react to the same event
	// ========================================================================

	sendEmailHandler := NewEventHandler(
		"OrderCreated",
		func(ctx context.Context, evt OrderCreated) error {
			log.Printf("📧 Sending confirmation email for order %s to customer %s", evt.ID, evt.CustomerID)

			// Send email (simulated)
			time.Sleep(100 * time.Millisecond)
			log.Printf("✉️  Email sent successfully")

			// Publish EmailSent event
			return eventBus.Publish(ctx, EmailSent{
				CustomerID: evt.CustomerID,
				OrderID:    evt.ID,
				Type:       "confirmation",
			})
		},
	)

	updateAnalyticsHandler := NewEventHandler(
		"OrderCreated",
		func(ctx context.Context, evt OrderCreated) error {
			log.Printf("📊 Updating analytics for order %s (amount: $%d)", evt.ID, evt.Amount)

			// Update analytics (simulated)
			time.Sleep(50 * time.Millisecond)
			log.Printf("📈 Analytics updated")

			return nil
		},
	)

	updateInventoryHandler := NewEventHandler(
		"OrderCreated",
		func(ctx context.Context, evt OrderCreated) error {
			log.Printf("📦 Updating inventory for order %s", evt.ID)

			// Update inventory (simulated)
			time.Sleep(75 * time.Millisecond)
			log.Printf("🔄 Inventory updated")

			return nil
		},
	)

	sendCancellationEmailHandler := NewEventHandler(
		"OrderCancelled",
		func(ctx context.Context, evt OrderCancelled) error {
			log.Printf("📧 Sending cancellation email for order %s", evt.ID)
			return nil
		},
	)

	// ========================================================================
	// Create Routers (Processors)
	// ========================================================================

	commandRouter := message.NewRouter(
		message.RouterConfig{
			Concurrency: 5,
			Recover:     true,
		},
		createOrderHandler,
		cancelOrderHandler,
	)

	eventRouter := message.NewRouter(
		message.RouterConfig{
			Concurrency: 10,
			Recover:     true,
		},
		sendEmailHandler,
		updateAnalyticsHandler,
		updateInventoryHandler,
		sendCancellationEmailHandler,
	)

	// Start processors
	commandOut := commandRouter.Start(ctx, commandChan)
	eventOut := eventRouter.Start(ctx, eventChan)

	// Drain outputs (handlers don't produce messages in this example)
	go func() {
		for range commandOut {
		}
	}()
	go func() {
		for range eventOut {
		}
	}()

	// ========================================================================
	// Demo: Send Commands and Watch Events Flow
	// ========================================================================

	log.Println(strings.Repeat("=", 70))
	log.Println("CQRS Example: Order Processing")
	log.Println(strings.Repeat("=", 70))
	log.Println()

	// Send CreateOrder command
	log.Println("🚀 Sending CreateOrder command...")
	err := commandBus.Send(ctx, CreateOrder{
		ID:         "order-123",
		CustomerID: "customer-456",
		Amount:     250,
	})
	if err != nil {
		log.Fatalf("Failed to send command: %v", err)
	}

	// Wait for event processing
	time.Sleep(500 * time.Millisecond)

	log.Println()
	log.Println("---")
	log.Println()

	// Send CancelOrder command
	log.Println("🚀 Sending CancelOrder command...")
	err = commandBus.Send(ctx, CancelOrder{
		ID:     "order-123",
		Reason: "Customer requested cancellation",
	})
	if err != nil {
		log.Fatalf("Failed to send command: %v", err)
	}

	// Wait for event processing
	time.Sleep(500 * time.Millisecond)

	log.Println()
	log.Println(strings.Repeat("=", 70))
	log.Println("Demo complete!")
	log.Println()
	log.Println("Key observations:")
	log.Println("  1. Commands are imperative (CreateOrder, CancelOrder)")
	log.Println("  2. Events are past tense (OrderCreated, OrderCancelled)")
	log.Println("  3. Single command handler per command")
	log.Println("  4. Multiple event handlers can react to same event")
	log.Println("  5. Clear separation of write (commands) and read (events)")
	log.Println(strings.Repeat("=", 70))
}
