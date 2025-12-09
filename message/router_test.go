package message_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

type Order struct {
	ID     string
	Amount int
}

type OrderConfirmed struct {
	ID          string
	ConfirmedAt time.Time
}

// testJSONHandler is a test helper that creates a handler with JSON marshaling.
// This is only used in tests; production code should use cqrs.NewCommandHandler.
func testJSONHandler[In, Out any](
	handle func(ctx context.Context, payload In) ([]Out, error),
	match func(prop message.Properties) bool,
	props func(prop message.Properties) message.Properties,
) message.Handler {
	h := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var payload In
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			err = fmt.Errorf("unmarshal message: %w: %w", message.ErrInvalidMessagePayload, err)
			msg.Nack(err)
			return nil, err
		}

		out, err := handle(ctx, payload)
		if err != nil {
			err = fmt.Errorf("handle message: %w", err)
			msg.Nack(err)
			return nil, err
		}

		var msgs []*message.Message
		for _, o := range out {
			data, err := json.Marshal(o)
			if err != nil {
				err = fmt.Errorf("marshal message: %w: %w", message.ErrInvalidMessagePayload, err)
				msg.Nack(err)
				return nil, err
			}
			outMsg := message.Copy(msg, data)
			outMsg.Properties = props(msg.Properties)
			msgs = append(msgs, outMsg)
		}

		msg.Ack()
		return msgs, nil
	}
	return message.NewHandler(h, match)
}

func TestRouter_BasicRouting(t *testing.T) {
	orderHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{
				ID:          order.ID,
				ConfirmedAt: time.Now(),
			}}, nil
		},
		func(prop message.Properties) bool {
			subject, _ := prop.Subject()
			return subject == "orders.new"
		},
		func(prop message.Properties) message.Properties {
			p := make(message.Properties)
			p[message.PropSubject] = "orders.confirmed"
			return p
		},
	)

	router := message.NewRouter(message.RouterConfig{}, orderHandler)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Properties{message.PropSubject: "orders.new"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if len(results) > 0 {
		subject, _ := results[0].Properties.Subject()
		if subject != "orders.confirmed" {
			t.Errorf("Expected subject 'orders.confirmed', got %s", subject)
		}

		var confirmed OrderConfirmed
		json.Unmarshal(results[0].Payload, &confirmed)
		if confirmed.ID != "order-1" {
			t.Errorf("Expected ID 'order-1', got %s", confirmed.ID)
		}
	}
}

func TestRouter_MultipleHandlers(t *testing.T) {
	orderHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			order.Amount *= 2
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			subject, _ := prop.Subject()
			return subject == "orders"
		},
		func(prop message.Properties) message.Properties {
			p := make(message.Properties)
			p[message.PropSubject] = "orders.processed"
			return p
		},
	)

	confirmHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		func(prop message.Properties) bool {
			subject, _ := prop.Subject()
			return subject == "confirmations"
		},
		func(prop message.Properties) message.Properties {
			p := make(message.Properties)
			p[message.PropSubject] = "confirmations.done"
			return p
		},
	)

	router := message.NewRouter(message.RouterConfig{}, orderHandler, confirmHandler)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 50})
	confirmData, _ := json.Marshal(Order{ID: "order-2", Amount: 75})

	in := channel.FromValues(
		message.New(orderData, message.Properties{message.PropSubject: "orders"}),
		message.New(confirmData, message.Properties{message.PropSubject: "confirmations"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]int)
	for msg := range out {
		subject, _ := msg.Properties.Subject()
		results[subject]++
	}

	if results["orders.processed"] != 1 {
		t.Errorf("Expected 1 order.processed, got %d", results["orders.processed"])
	}
	if results["confirmations.done"] != 1 {
		t.Errorf("Expected 1 confirmation.done, got %d", results["confirmations.done"])
	}
}

func TestRouter_NoMatchingHandler(t *testing.T) {
	orderHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			subject, _ := prop.Subject()
			return subject == "orders"
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, orderHandler)

	// Message with non-matching subject
	data, _ := json.Marshal(Order{ID: "order-1"})
	var nackCalled bool
	var nackErr error

	in := channel.FromValues(
		message.NewWithAcking(data, message.Properties{
			message.PropSubject: "unknown",
		},
			func() {},
			func(err error) {
				nackCalled = true
				nackErr = err
			}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 results for no match, got %d", count)
	}

	if !nackCalled {
		t.Error("Expected nack to be called")
	}
	if nackErr == nil {
		t.Error("Expected nack error to be set")
	}
}

func TestRouter_HandlerError(t *testing.T) {
	testErr := errors.New("handler error")
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return nil, testErr
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)

	data, _ := json.Marshal(Order{ID: "order-1"})
	var nackCalled bool
	var nackErr error

	in := channel.FromValues(
		message.NewWithAcking(data, nil,
			func() {},
			func(err error) {
				nackCalled = true
				nackErr = err
			}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 results for error, got %d", count)
	}

	if !nackCalled {
		t.Error("Expected nack to be called")
	}
	if nackErr == nil {
		t.Error("Expected nack error to be set")
	}
}

func TestRouter_UnmarshalError(t *testing.T) {
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)

	var nackCalled bool
	var nackErr error

	in := channel.FromValues(
		message.NewWithAcking([]byte("invalid json"), nil,
			func() {},
			func(err error) {
				nackCalled = true
				nackErr = err
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
	if nackErr == nil || !errors.Is(nackErr, message.ErrInvalidMessagePayload) {
		t.Errorf("Expected ErrInvalidMessagePayload, got %v", nackErr)
	}
}

func TestRouter_AckOnSuccess(t *testing.T) {
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)

	data, _ := json.Marshal(Order{ID: "order-1"})
	var ackCalled bool

	in := channel.FromValues(
		message.NewWithAcking(data, nil,
			func() {
				ackCalled = true
			},
			func(err error) {}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for range out {
	}

	if !ackCalled {
		t.Error("Expected ack to be called on success")
	}
}

func TestRouter_Concurrency(t *testing.T) {
	var mu sync.Mutex
	var processedIDs []string

	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			processedIDs = append(processedIDs, order.ID)
			mu.Unlock()
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{
		Concurrency: 5,
	}, handler)

	in := make(chan *message.Message, 10)
	for i := 0; i < 10; i++ {
		data, _ := json.Marshal(Order{ID: string(rune('A' + i))})
		in <- message.New(data, nil)
	}
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 10 {
		t.Errorf("Expected 10 results, got %d", count)
	}

	mu.Lock()
	if len(processedIDs) != 10 {
		t.Errorf("Expected 10 processed IDs, got %d", len(processedIDs))
	}
	mu.Unlock()
}

func TestRouter_WithRecover(t *testing.T) {
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			panic("handler panic")
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{
		Recover: true,
	}, handler)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(message.New(data, nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	// Should not panic due to Recover option
	for range out {
	}
}

func TestRouter_MultipleOutputMessages(t *testing.T) {
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			// Return multiple messages
			return []Order{
				{ID: order.ID + "-1", Amount: order.Amount},
				{ID: order.ID + "-2", Amount: order.Amount},
			}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)

	data, _ := json.Marshal(Order{ID: "order", Amount: 100})
	in := channel.FromValues(message.New(data, nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 output messages, got %d", count)
	}
}

func TestRouter_PreservesProperties(t *testing.T) {
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			// Preserve correlation ID
			p := make(message.Properties)
			if corr, ok := prop.CorrelationID(); ok {
				p[message.PropCorrelationID] = corr
			}
			p[message.PropSubject] = "processed"
			return p
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(
		message.New(data, message.Properties{
			message.PropCorrelationID: "corr-123",
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for msg := range out {
		corrID, ok := msg.Properties.CorrelationID()
		if !ok || corrID != "corr-123" {
			t.Errorf("Expected correlation ID 'corr-123', got %s (ok=%v)", corrID, ok)
		}
		subject, ok := msg.Properties.Subject()
		if !ok || subject != "processed" {
			t.Errorf("Expected subject 'processed', got %s (ok=%v)", subject, ok)
		}
	}
}

func TestRouter_Middleware(t *testing.T) {
	var executionOrder []string
	mu := &sync.Mutex{}

	// Middleware that records execution order
	middleware1 := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "middleware1-before")
			mu.Unlock()
			result, err := next(ctx, msg)
			mu.Lock()
			executionOrder = append(executionOrder, "middleware1-after")
			mu.Unlock()
			return result, err
		}
	}

	middleware2 := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "middleware2-before")
			mu.Unlock()
			result, err := next(ctx, msg)
			mu.Lock()
			executionOrder = append(executionOrder, "middleware2-after")
			mu.Unlock()
			return result, err
		}
	}

	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "handler")
			mu.Unlock()
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)
	router.AddMiddleware(middleware1, middleware2)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(message.New(data, nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)
	for range out {
	}

	expected := []string{
		"middleware1-before",
		"middleware2-before",
		"handler",
		"middleware2-after",
		"middleware1-after",
	}

	mu.Lock()
	defer mu.Unlock()
	if len(executionOrder) != len(expected) {
		t.Errorf("Expected %d steps, got %d: %v", len(expected), len(executionOrder), executionOrder)
		return
	}

	for i, step := range expected {
		if executionOrder[i] != step {
			t.Errorf("Step %d: expected %s, got %s", i, step, executionOrder[i])
		}
	}
}

func TestRouter_MiddlewareModifyMessage(t *testing.T) {
	// Middleware that adds a property
	addPropertyMiddleware := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			msg.Properties["middleware-added"] = "yes"
			return next(ctx, msg)
		}
	}

	var receivedProps message.Properties
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			receivedProps = prop
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)
	router.AddMiddleware(addPropertyMiddleware)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(message.New(data, nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)
	for range out {
	}

	if receivedProps["middleware-added"] != "yes" {
		t.Errorf("Expected middleware to add property, got: %v", receivedProps)
	}
}

func TestRouter_MiddlewareShortCircuit(t *testing.T) {
	// Middleware that short-circuits the handler
	shortCircuitMiddleware := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			if msg.Properties["bypass"] == "yes" {
				msg.Ack()
				return nil, nil
			}
			return next(ctx, msg)
		}
	}

	var handlerCalled bool
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			handlerCalled = true
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)
	router.AddMiddleware(shortCircuitMiddleware)

	data, _ := json.Marshal(Order{ID: "order-1"})
	var ackCalled bool
	in := channel.FromValues(
		message.NewWithAcking(data, message.Properties{"bypass": "yes"},
			func() { ackCalled = true },
			func(err error) {}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)
	for range out {
	}

	if handlerCalled {
		t.Error("Expected handler not to be called due to short-circuit")
	}
	if !ackCalled {
		t.Error("Expected message to be acked by middleware")
	}
}

func TestRouter_MiddlewareError(t *testing.T) {
	testErr := errors.New("middleware error")
	// Middleware that returns an error
	errorMiddleware := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			msg.Nack(testErr)
			return nil, testErr
		}
	}

	var handlerCalled bool
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			handlerCalled = true
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)
	router.AddMiddleware(errorMiddleware)

	data, _ := json.Marshal(Order{ID: "order-1"})
	var nackCalled bool
	var nackErr error
	in := channel.FromValues(
		message.NewWithAcking(data, nil,
			func() {},
			func(err error) {
				nackCalled = true
				nackErr = err
			}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)
	for range out {
	}

	if handlerCalled {
		t.Error("Expected handler not to be called due to middleware error")
	}
	if !nackCalled {
		t.Error("Expected message to be nacked by middleware")
	}
	if !errors.Is(nackErr, testErr) {
		t.Errorf("Expected error %v, got %v", testErr, nackErr)
	}
}

func TestRouter_MultipleMiddlewares(t *testing.T) {
	// Test that multiple middlewares can be added at once
	var calls []string
	mu := &sync.Mutex{}

	m1 := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			mu.Lock()
			calls = append(calls, "m1")
			mu.Unlock()
			return next(ctx, msg)
		}
	}

	m2 := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			mu.Lock()
			calls = append(calls, "m2")
			mu.Unlock()
			return next(ctx, msg)
		}
	}

	m3 := func(next message.HandlerFunc) message.HandlerFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			mu.Lock()
			calls = append(calls, "m3")
			mu.Unlock()
			return next(ctx, msg)
		}
	}

	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Properties) bool {
			return true
		},
		func(prop message.Properties) message.Properties {
			return make(message.Properties)
		},
	)

	router := message.NewRouter(message.RouterConfig{}, handler)
	router.AddMiddleware(m1, m2, m3)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(message.New(data, nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)
	for range out {
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"m1", "m2", "m3"}
	if len(calls) != len(expected) {
		t.Errorf("Expected %d middleware calls, got %d", len(expected), len(calls))
		return
	}

	for i, call := range expected {
		if calls[i] != call {
			t.Errorf("Call %d: expected %s, got %s", i, call, calls[i])
		}
	}
}
