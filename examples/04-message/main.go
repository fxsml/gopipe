// Example: Message routing with CloudEvents handlers.
//
// Demonstrates the core message package concepts:
// - Creating a Router and registering a command handler
// - Composing raw ([]byte) I/O via UnmarshalPipe/MarshalPipe
// - Message acknowledgment (acking)
// - A pure, heterogeneous-output command handler pattern with middleware
//
// Run: go run ./examples/04-message
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"reflect"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/middleware"
	"github.com/google/uuid"
)

// CreateOrder is the input command type.
type CreateOrder struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// OrderCreated is the output event type.
type OrderCreated struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	basicRouting()
	pureHeterogeneousHandler()
}

// basicRouting demonstrates a Router composed with UnmarshalPipe/MarshalPipe
// for raw ([]byte) I/O — the pattern for broker integration.
func basicRouting() {
	router := message.NewRouter(message.PipeConfig{})

	// Register a command handler.
	// Input type (CreateOrder) determines which messages it handles.
	// Output type (OrderCreated) determines the response event type.
	handler := message.NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
			fmt.Printf("Processing order: %s (amount: %.2f)\n", cmd.OrderID, cmd.Amount)
			return []OrderCreated{{
				OrderID: cmd.OrderID,
				Status:  "created",
			}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/orders",
			Naming: message.DotNaming, // CreateOrder → "create.order"
		},
	)
	router.AddHandler("process-order", nil, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Raw input → typed, via unmarshal pipe.
	input := make(chan *message.RawMessage, 10)
	marshaler := message.NewJSONMarshaler()
	unmarshal := message.NewUnmarshalPipe(router, marshaler, message.PipeConfig{})
	typedIn, _ := unmarshal.Pipe(ctx, input)

	typedOut, _ := router.Pipe(ctx, typedIn)

	// Typed output → raw, via marshal pipe.
	marshal := message.NewMarshalPipe(marshaler, message.PipeConfig{})
	output, _ := marshal.Pipe(ctx, typedOut)

	// Send a message with acking.
	data, err := json.Marshal(CreateOrder{OrderID: "ORD-123", Amount: 99.99})
	if err != nil {
		log.Fatal(err)
	}
	input <- message.NewRaw(data, message.Attributes{
		message.AttrSpecVersion: "1.0",
		message.AttrType:        "create.order",
		message.AttrSource:      "/test",
		message.AttrID:          uuid.NewString(),
	}, message.NewAcking(
		func() { fmt.Println("Message successfully processed") },
		func(err error) { fmt.Printf("Message processing failed: %v\n", err) },
	))

	// Receive the response.
	result := <-output
	fmt.Printf("Received: %s\n", result)

	close(input)
	cancel()

	// Output (ack timing relative to "Received" may vary):
	// Processing order: ORD-123 (amount: 99.99)
	// Message successfully processed
	// Received: {"data":{"order_id":"ORD-123","status":"created"},...}
}

// ExpiredEvent is the input event type for the heterogeneous handler demo.
type ExpiredEvent struct{ ArticleID string }

// SeedArticleCmd is one of two command types produced from a single ExpiredEvent.
type SeedArticleCmd struct{ ArticleID string }

// SeedAvailabilityCmd is the other command type produced from a single ExpiredEvent.
type SeedAvailabilityCmd struct{ ArticleID string }

// transformExpired is pure (no ctx, cannot fail) and heterogeneous
// (fans out to two different command types).
func transformExpired(e ExpiredEvent) []any {
	return []any{
		SeedArticleCmd{ArticleID: e.ArticleID},
		SeedAvailabilityCmd{ArticleID: e.ArticleID},
	}
}

// pureHeterogeneousHandler demonstrates NewTransformHandler: a documented pattern
// (not new package API) for pure, potentially heterogeneous-output event-to-command
// translation, composed with Router and middleware via .Use().
func pureHeterogeneousHandler() {
	fmt.Println()
	fmt.Println("--- Pure/heterogeneous command handler ---")

	router := message.NewRouter(message.PipeConfig{})
	handler := NewTransformHandler(transformExpired, message.CommandHandlerConfig{
		Source: "/seeding",
		Naming: message.DotNaming, // ExpiredEvent → "expired.event"
	})
	router.AddHandler("expired-transform", nil, handler)
	_ = router.Use(middleware.CorrelationID())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan *message.Message, 1)
	out, _ := router.Pipe(ctx, input)

	input <- &message.Message{
		Data: ExpiredEvent{ArticleID: "ART-1"},
		Attributes: message.Attributes{
			message.AttrType:          "expired.event",
			message.AttrCorrelationID: "corr-123",
		},
	}
	close(input)

	for msg := range out {
		fmt.Printf("Produced %T (correlationid=%v): %+v\n",
			msg.Data, msg.Attributes[message.AttrCorrelationID], msg.Data)
	}
}

// NewTransformHandler builds a Handler for pure, potentially heterogeneous-output
// event-to-command translation: fn cannot fail, needs no context, and each output
// value gets its own CE type derived via naming — E may be a concrete type
// (homogeneous output) or `any` (heterogeneous output), both work identically.
func NewTransformHandler[C, E any](
	fn func(C) []E,
	cfg message.CommandHandlerConfig,
) message.Handler {
	naming := cfg.Naming
	if naming == nil {
		naming = message.DefaultNaming
	}
	return message.NewHandler[C](func(_ context.Context, msg *message.Message) ([]*message.Message, error) {
		var cmd C
		switch v := msg.Data.(type) {
		case *C:
			cmd = *v
		case C:
			cmd = v
		default:
			return nil, fmt.Errorf("unexpected data type %T, want %T", msg.Data, cmd)
		}

		events := fn(cmd)
		outputs := make([]*message.Message, len(events))
		for i, event := range events {
			attrs := message.Attributes{
				message.AttrID:          message.NewID(),
				message.AttrSpecVersion: "1.0",
				message.AttrType:        naming.EventType(elemType(event)),
				message.AttrSource:      cfg.Source,
				message.AttrTime:        time.Now().UTC(),
			}
			maps.Copy(attrs, cfg.Attributes)
			outputs[i] = message.New(event, attrs, nil)
		}
		return outputs, nil
	}, naming)
}

func elemType(v any) reflect.Type {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
