package cqrs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/cqrs"
)

// Test types
type Order struct {
	ID     string
	Amount int
}

type OrderConfirmed struct {
	ID          string
	ConfirmedAt time.Time
}

func TestRouter_CommandHandler(t *testing.T) {
	marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

	handler := cqrs.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{
				ID:          order.ID,
				ConfirmedAt: time.Now(),
			}}, nil
		},
		cqrs.Match(cqrs.MatchSubject("Order"), cqrs.MatchType("command")),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{
			message.AttrSubject: "Order",
			message.AttrType:    "command",
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	// WithTypeOf() sets type to the reflected type name
	msgType, _ := results[0].Attributes.Type()
	if msgType != "OrderConfirmed" {
		t.Errorf("Expected type 'OrderConfirmed', got %s", msgType)
	}

	var confirmed OrderConfirmed
	if err := json.Unmarshal(results[0].Data, &confirmed); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}
	if confirmed.ID != "order-1" {
		t.Errorf("Expected ID 'order-1', got %s", confirmed.ID)
	}
}

func TestRouter_CommandHandler_WithCustomAttrs(t *testing.T) {
	// Test with custom attribute providers for subject and type
	marshaler := cqrs.NewJSONCommandMarshaler(
		cqrs.CombineAttrs(
			cqrs.WithTypeOf(),                       // type = "OrderConfirmed"
			cqrs.WithAttribute("category", "event"), // custom attribute
		),
	)

	handler := cqrs.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		cqrs.MatchSubject("Order"),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	orderData, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "Order"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	msgType, _ := results[0].Attributes.Type()
	if msgType != "OrderConfirmed" {
		t.Errorf("Expected type 'OrderConfirmed', got %s", msgType)
	}

	category, ok := results[0].Attributes["category"]
	if !ok || category != "event" {
		t.Errorf("Expected category 'event', got %v", category)
	}
}

func TestRouter_EventHandler(t *testing.T) {
	marshaler := cqrs.NewJSONEventMarshaler()

	var processed bool
	handler := cqrs.NewEventHandler(
		func(ctx context.Context, evt OrderConfirmed) error {
			processed = true
			if evt.ID != "order-1" {
				t.Errorf("Expected ID 'order-1', got %s", evt.ID)
			}
			return nil
		},
		cqrs.Match(cqrs.MatchSubject("OrderConfirmed"), cqrs.MatchType("event")),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	eventData, _ := json.Marshal(OrderConfirmed{ID: "order-1", ConfirmedAt: time.Now()})
	in := channel.FromValues(
		message.New(eventData, message.Attributes{
			message.AttrSubject: "OrderConfirmed",
			message.AttrType:    "event",
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for range out {
	}

	if !processed {
		t.Error("Expected event to be processed")
	}
}

func TestRouter_UnmarshalError(t *testing.T) {
	marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

	handler := cqrs.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		cqrs.Match(cqrs.MatchSubject("Order")),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	var nackCalled bool

	in := channel.FromValues(
		message.NewWithAcking([]byte("invalid json"), message.Attributes{
			message.AttrSubject: "Order",
		},
			func() {},
			func(err error) {
				nackCalled = true
			}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for range out {
	}

	if !nackCalled {
		t.Error("Expected nack to be called for unmarshal error")
	}
}

func TestRouter_HandlerError(t *testing.T) {
	marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())
	testErr := errors.New("handler error")

	handler := cqrs.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return nil, testErr
		},
		cqrs.Match(cqrs.MatchSubject("Order")),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	var nackCalled bool

	orderData, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(
		message.NewWithAcking(orderData, message.Attributes{
			message.AttrSubject: "Order",
		},
			func() {},
			func(err error) {
				nackCalled = true
			}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for range out {
	}

	if !nackCalled {
		t.Error("Expected nack to be called")
	}
}

func TestRouter_MultipleCommandHandlers(t *testing.T) {
	marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

	orderHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		cqrs.Match(cqrs.MatchSubject("Order")),
		marshaler,
	)

	type Payment struct{ ID string }
	type PaymentProcessed struct{ ID string }

	paymentHandler := cqrs.NewCommandHandler(
		func(ctx context.Context, payment Payment) ([]PaymentProcessed, error) {
			return []PaymentProcessed{{ID: payment.ID}}, nil
		},
		cqrs.Match(cqrs.MatchSubject("Payment")),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(orderHandler)
	router.AddHandler(paymentHandler)

	orderData, _ := json.Marshal(Order{ID: "order-1"})
	paymentData, _ := json.Marshal(Payment{ID: "payment-1"})

	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "Order"}),
		message.New(paymentData, message.Attributes{message.AttrSubject: "Payment"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	// WithTypeOf() sets type to reflected type name
	results := make(map[string]int)
	for msg := range out {
		msgType, _ := msg.Attributes.Type()
		results[msgType]++
	}

	if results["OrderConfirmed"] != 1 {
		t.Errorf("Expected 1 OrderConfirmed, got %d", results["OrderConfirmed"])
	}
	if results["PaymentProcessed"] != 1 {
		t.Errorf("Expected 1 PaymentProcessed, got %d", results["PaymentProcessed"])
	}
}

func TestRouter_WithCommandPipe(t *testing.T) {
	marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

	// Create a typed pipe that doubles the order amount
	pipe := gopipe.NewTransformPipe(func(ctx context.Context, order Order) (Order, error) {
		order.Amount *= 2
		return order, nil
	})

	commandPipe := cqrs.NewCommandPipe(pipe, cqrs.MatchSubject("Order"), marshaler)

	router := message.NewRouter(message.RouterConfig{})
	router.AddPipe(commandPipe)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "Order"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	var order Order
	if err := json.Unmarshal(results[0].Data, &order); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}
	if order.Amount != 200 {
		t.Errorf("Expected amount 200, got %d", order.Amount)
	}
}

func TestRouter_PipeWithHandlerFallback(t *testing.T) {
	marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

	// Pipe for "double" operations
	doublePipe := gopipe.NewTransformPipe(func(ctx context.Context, order Order) (Order, error) {
		order.Amount *= 2
		return order, nil
	})
	commandPipe := cqrs.NewCommandPipe(doublePipe, cqrs.MatchSubject("DoubleOrder"), marshaler)

	// Handler for regular orders
	handler := cqrs.NewCommandHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		cqrs.Match(cqrs.MatchSubject("Order")),
		marshaler,
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddPipe(commandPipe)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	doubleData, _ := json.Marshal(Order{ID: "order-2", Amount: 50})

	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "Order"}),
		message.New(doubleData, message.Attributes{message.AttrSubject: "DoubleOrder"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var orderConfirmed, orderDoubled int
	for msg := range out {
		msgType, _ := msg.Attributes.Type()
		switch msgType {
		case "OrderConfirmed":
			orderConfirmed++
		case "Order":
			var o Order
			if err := json.Unmarshal(msg.Data, &o); err != nil {
				t.Errorf("Failed to unmarshal Order: %v", err)
				continue
			}
			if o.Amount == 100 {
				orderDoubled++
			}
		}
	}

	if orderConfirmed != 1 {
		t.Errorf("Expected 1 OrderConfirmed, got %d", orderConfirmed)
	}
	if orderDoubled != 1 {
		t.Errorf("Expected 1 doubled order, got %d", orderDoubled)
	}
}
