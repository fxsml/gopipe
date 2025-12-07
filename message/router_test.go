package message_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestRouter_BasicRouting(t *testing.T) {
	orderHandler := message.NewHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{
				ID:          order.ID,
				ConfirmedAt: time.Now(),
			}}, nil
		},
		func(prop map[string]any) bool {
			subject, _ := message.SubjectProps(prop)
			return subject == "orders.new"
		},
		func(prop map[string]any) map[string]any {
			p := make(map[string]any)
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
		subject, _ := message.SubjectProps(results[0].Properties)
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
	orderHandler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			order.Amount *= 2
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			subject, _ := message.SubjectProps(prop)
			return subject == "orders"
		},
		func(prop map[string]any) map[string]any {
			p := make(map[string]any)
			p[message.PropSubject] = "orders.processed"
			return p
		},
	)

	confirmHandler := message.NewHandler(
		func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
			return []OrderConfirmed{{ID: order.ID}}, nil
		},
		func(prop map[string]any) bool {
			subject, _ := message.SubjectProps(prop)
			return subject == "confirmations"
		},
		func(prop map[string]any) map[string]any {
			p := make(map[string]any)
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
		subject, _ := message.SubjectProps(msg.Properties)
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
	orderHandler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			subject, _ := message.SubjectProps(prop)
			return subject == "orders"
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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
	if nackErr == nil || !errors.Is(nackErr, message.ErrInvalidMessageProperties) {
		t.Errorf("Expected ErrInvalidMessageProperties, got %v", nackErr)
	}
}

func TestRouter_HandlerError(t *testing.T) {
	testErr := errors.New("handler error")
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return nil, testErr
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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

	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			processedIDs = append(processedIDs, order.ID)
			mu.Unlock()
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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

func TestRouter_CustomMarshalUnmarshal(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
		},
	)

	customMarshalCalled := false
	customUnmarshalCalled := false

	router := message.NewRouter(message.RouterConfig{
		Marshal: func(msg any) ([]byte, error) {
			customMarshalCalled = true
			return json.Marshal(msg)
		},
		Unmarshal: func(data []byte, msg any) error {
			customUnmarshalCalled = true
			return json.Unmarshal(data, msg)
		},
	}, handler)

	data, _ := json.Marshal(Order{ID: "order-1"})
	in := channel.FromValues(message.New(data, nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	for range out {
	}

	if !customMarshalCalled {
		t.Error("Expected custom marshal to be called")
	}
	if !customUnmarshalCalled {
		t.Error("Expected custom unmarshal to be called")
	}
}

func TestRouter_WithRecover(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			panic("handler panic")
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			// Return multiple messages
			return []Order{
				{ID: order.ID + "-1", Amount: order.Amount},
				{ID: order.ID + "-2", Amount: order.Amount},
			}, nil
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			return make(map[string]any)
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
	handler := message.NewHandler(
		func(ctx context.Context, order Order) ([]Order, error) {
			return []Order{order}, nil
		},
		func(prop map[string]any) bool {
			return true
		},
		func(prop map[string]any) map[string]any {
			// Preserve correlation ID
			p := make(map[string]any)
			if corr, ok := message.CorrelationIDProps(prop); ok {
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
		corrID, ok := message.CorrelationIDProps(msg.Properties)
		if !ok || corrID != "corr-123" {
			t.Errorf("Expected correlation ID 'corr-123', got %s (ok=%v)", corrID, ok)
		}
		subject, ok := message.SubjectProps(msg.Properties)
		if !ok || subject != "processed" {
			t.Errorf("Expected subject 'processed', got %s (ok=%v)", subject, ok)
		}
	}
}
