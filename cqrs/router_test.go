package cqrs_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/cqrs"
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
	match func(prop message.Attributes) bool,
	props func(prop message.Attributes) message.Attributes,
) cqrs.Handler {
	h := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var payload In
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			err = fmt.Errorf("unmarshal message: %w: %w", cqrs.ErrInvalidMessagePayload, err)
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
				err = fmt.Errorf("marshal message: %w: %w", cqrs.ErrInvalidMessagePayload, err)
				msg.Nack(err)
				return nil, err
			}
			outMsg := message.Copy(msg, data)
			outMsg.Attributes = props(msg.Attributes)
			msgs = append(msgs, outMsg)
		}

		msg.Ack()
		return msgs, nil
	}
	return cqrs.NewHandler(h, match)
}

func TestRouter_BasicRouting(t *testing.T) {
	orderHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{
				ID:          order.ID,
				ConfirmedAt: time.Now(),
			}}, nil
		},
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			return subject == "orders.new"
		},
		func(prop message.Attributes) message.Attributes {
			p := make(message.Attributes)
			p[message.AttrSubject] = "orders.confirmed"
			return p
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, orderHandler)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "orders.new"}),
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
		subject, _ := results[0].Attributes.Subject()
		if subject != "orders.confirmed" {
			t.Errorf("Expected subject 'orders.confirmed', got %s", subject)
		}

		var confirmed OrderConfirmed
		json.Unmarshal(results[0].Data, &confirmed)
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
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			return subject == "orders"
		},
		func(prop message.Attributes) message.Attributes {
			p := make(message.Attributes)
			p[message.AttrSubject] = "orders.processed"
			return p
		},
	)

	confirmHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			return subject == "confirmations"
		},
		func(prop message.Attributes) message.Attributes {
			p := make(message.Attributes)
			p[message.AttrSubject] = "confirmations.done"
			return p
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, orderHandler, confirmHandler)

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 50})
	confirmData, _ := json.Marshal(Order{ID: "order-2", Amount: 75})

	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "orders"}),
		message.New(confirmData, message.Attributes{message.AttrSubject: "confirmations"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]int)
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
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
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			return subject == "orders"
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, orderHandler)

	// Message with non-matching subject
	data, _ := json.Marshal(Order{ID: "order-1"})
	var nackCalled bool
	var nackErr error

	in := channel.FromValues(
		message.NewWithAcking(data, message.Attributes{
			message.AttrSubject: "unknown",
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
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, handler)

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
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, handler)

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
	if nackErr == nil || !errors.Is(nackErr, cqrs.ErrInvalidMessagePayload) {
		t.Errorf("Expected ErrInvalidMessagePayload, got %v", nackErr)
	}
}

func TestRouter_AckOnSuccess(t *testing.T) {
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, handler)

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
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{
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
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{
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
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			return make(message.Attributes)
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, handler)

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
		func(prop message.Attributes) bool {
			return true
		},
		func(prop message.Attributes) message.Attributes {
			// Preserve correlation ID
			p := make(message.Attributes)
			if corr, ok := prop.CorrelationID(); ok {
				p[message.AttrCorrelationID] = corr
			}
			p[message.AttrSubject] = "processed"
			return p
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, handler)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(
		message.New(data, message.Attributes{
			message.AttrCorrelationID: "corr-123",
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for msg := range out {
		corrID, ok := msg.Attributes.CorrelationID()
		if !ok || corrID != "corr-123" {
			t.Errorf("Expected correlation ID 'corr-123', got %s (ok=%v)", corrID, ok)
		}
		subject, ok := msg.Attributes.Subject()
		if !ok || subject != "processed" {
			t.Errorf("Expected subject 'processed', got %s (ok=%v)", subject, ok)
		}
	}
}

func TestRouter_AddPipe_Basic(t *testing.T) {
	// Create a simple pipe that doubles the message amount
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var order Order
		if err := json.Unmarshal(msg.Data, &order); err != nil {
			return nil, err
		}
		order.Amount *= 2
		data, _ := json.Marshal(order)
		outMsg := message.Copy(msg, data)
		outMsg.Attributes = make(message.Attributes)
		outMsg.Attributes[message.AttrSubject] = "orders.doubled"
		return []*message.Message{outMsg}, nil
	})

	router := cqrs.NewRouter(cqrs.RouterConfig{})
	router.AddPipe(pipe, func(prop message.Attributes) bool {
		subject, _ := prop.Subject()
		return subject == "orders.double"
	})

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "orders.double"}),
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

	subject, _ := results[0].Attributes.Subject()
	if subject != "orders.doubled" {
		t.Errorf("Expected subject 'orders.doubled', got %s", subject)
	}

	var order Order
	json.Unmarshal(results[0].Data, &order)
	if order.Amount != 200 {
		t.Errorf("Expected amount 200, got %d", order.Amount)
	}
}

func TestRouter_AddPipe_WithHandlers(t *testing.T) {
	// Create a pipe for "orders.double" messages
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var order Order
		if err := json.Unmarshal(msg.Data, &order); err != nil {
			return nil, err
		}
		order.Amount *= 2
		data, _ := json.Marshal(order)
		outMsg := message.Copy(msg, data)
		outMsg.Attributes = make(message.Attributes)
		outMsg.Attributes[message.AttrSubject] = "orders.doubled"
		return []*message.Message{outMsg}, nil
	})

	// Create a handler for "orders.confirm" messages
	confirmHandler := testJSONHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID, ConfirmedAt: time.Now()}}, nil
		},
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			return subject == "orders.confirm"
		},
		func(prop message.Attributes) message.Attributes {
			p := make(message.Attributes)
			p[message.AttrSubject] = "orders.confirmed"
			return p
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, confirmHandler)
	router.AddPipe(pipe, func(prop message.Attributes) bool {
		subject, _ := prop.Subject()
		return subject == "orders.double"
	})

	doubleData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	confirmData, _ := json.Marshal(Order{ID: "order-2", Amount: 50})

	in := channel.FromValues(
		message.New(doubleData, message.Attributes{message.AttrSubject: "orders.double"}),
		message.New(confirmData, message.Attributes{message.AttrSubject: "orders.confirm"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]int)
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		results[subject]++
	}

	if results["orders.doubled"] != 1 {
		t.Errorf("Expected 1 orders.doubled, got %d", results["orders.doubled"])
	}
	if results["orders.confirmed"] != 1 {
		t.Errorf("Expected 1 orders.confirmed, got %d", results["orders.confirmed"])
	}
}

func TestRouter_AddPipe_MultiplePipes(t *testing.T) {
	// Pipe 1: doubles the amount
	doublePipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var order Order
		if err := json.Unmarshal(msg.Data, &order); err != nil {
			return nil, err
		}
		order.Amount *= 2
		data, _ := json.Marshal(order)
		outMsg := message.Copy(msg, data)
		outMsg.Attributes = make(message.Attributes)
		outMsg.Attributes[message.AttrSubject] = "orders.doubled"
		return []*message.Message{outMsg}, nil
	})

	// Pipe 2: triples the amount
	triplePipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var order Order
		if err := json.Unmarshal(msg.Data, &order); err != nil {
			return nil, err
		}
		order.Amount *= 3
		data, _ := json.Marshal(order)
		outMsg := message.Copy(msg, data)
		outMsg.Attributes = make(message.Attributes)
		outMsg.Attributes[message.AttrSubject] = "orders.tripled"
		return []*message.Message{outMsg}, nil
	})

	router := cqrs.NewRouter(cqrs.RouterConfig{})
	router.AddPipe(doublePipe, func(prop message.Attributes) bool {
		subject, _ := prop.Subject()
		return subject == "orders.double"
	})
	router.AddPipe(triplePipe, func(prop message.Attributes) bool {
		subject, _ := prop.Subject()
		return subject == "orders.triple"
	})

	doubleData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	tripleData, _ := json.Marshal(Order{ID: "order-2", Amount: 100})

	in := channel.FromValues(
		message.New(doubleData, message.Attributes{message.AttrSubject: "orders.double"}),
		message.New(tripleData, message.Attributes{message.AttrSubject: "orders.triple"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]*Order)
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		var order Order
		json.Unmarshal(msg.Data, &order)
		results[subject] = &order
	}

	if doubledOrder, ok := results["orders.doubled"]; !ok {
		t.Error("Expected orders.doubled result")
	} else if doubledOrder.Amount != 200 {
		t.Errorf("Expected doubled amount 200, got %d", doubledOrder.Amount)
	}

	if tripledOrder, ok := results["orders.tripled"]; !ok {
		t.Error("Expected orders.tripled result")
	} else if tripledOrder.Amount != 300 {
		t.Errorf("Expected tripled amount 300, got %d", tripledOrder.Amount)
	}
}

func TestRouter_AddPipe_NoMatch(t *testing.T) {
	// Create a pipe that should NOT match
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		t.Error("Pipe should not be called for non-matching message")
		return []*message.Message{msg}, nil
	})

	// Create a handler that SHOULD match
	handler := testJSONHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop message.Attributes) bool {
			subject, _ := prop.Subject()
			return subject == "orders.process"
		},
		func(prop message.Attributes) message.Attributes {
			p := make(message.Attributes)
			p[message.AttrSubject] = "orders.processed"
			return p
		},
	)

	router := cqrs.NewRouter(cqrs.RouterConfig{}, handler)
	router.AddPipe(pipe, func(prop message.Attributes) bool {
		subject, _ := prop.Subject()
		return subject == "orders.double"
	})

	orderData, _ := json.Marshal(Order{ID: "order-1", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "orders.process"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		if subject != "orders.processed" {
			t.Errorf("Expected subject 'orders.processed', got %s", subject)
		}
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 result, got %d", count)
	}
}

func TestRouter_AddPipe_MultipleOutputs(t *testing.T) {
	// Create a pipe that produces multiple outputs
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		var order Order
		if err := json.Unmarshal(msg.Data, &order); err != nil {
			return nil, err
		}

		// Create two output messages
		out1 := Order{ID: order.ID + "-1", Amount: order.Amount * 2}
		out2 := Order{ID: order.ID + "-2", Amount: order.Amount * 3}

		data1, _ := json.Marshal(out1)
		data2, _ := json.Marshal(out2)

		msg1 := message.Copy(msg, data1)
		msg1.Attributes = make(message.Attributes)
		msg1.Attributes[message.AttrSubject] = "orders.split"

		msg2 := message.Copy(msg, data2)
		msg2.Attributes = make(message.Attributes)
		msg2.Attributes[message.AttrSubject] = "orders.split"

		return []*message.Message{msg1, msg2}, nil
	})

	router := cqrs.NewRouter(cqrs.RouterConfig{})
	router.AddPipe(pipe, func(prop message.Attributes) bool {
		subject, _ := prop.Subject()
		return subject == "orders.split"
	})

	orderData, _ := json.Marshal(Order{ID: "order", Amount: 100})
	in := channel.FromValues(
		message.New(orderData, message.Attributes{message.AttrSubject: "orders.split"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
}
