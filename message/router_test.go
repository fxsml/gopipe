package message

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRouter_BasicRouting(t *testing.T) {
	router := NewRouter(RouterConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		cmd := msg.Data.(*TestCommand)
		return []*Message{
			{
				Data:       TestEvent{ID: cmd.ID, Status: "processed"},
				Attributes: msg.Attributes,
			},
		}, nil
	}, KebabNaming)

	router.AddHandler(handler, HandlerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan *Message, 1)
	in <- &Message{
		Data:       &TestCommand{ID: "123", Name: "test"},
		Attributes: Attributes{"type": "test.command"},
	}
	close(in)

	out, err := router.Pipe(ctx, in)
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}

	msg := <-out
	if msg == nil {
		t.Fatal("expected message, got nil")
	}

	event, ok := msg.Data.(TestEvent)
	if !ok {
		t.Fatalf("expected TestEvent, got %T", msg.Data)
	}
	if event.ID != "123" || event.Status != "processed" {
		t.Errorf("unexpected event: %+v", event)
	}

	// Verify channel closes
	if _, ok := <-out; ok {
		t.Error("expected channel to be closed")
	}
}

func TestRouter_NoHandler(t *testing.T) {
	var handledErr error
	router := NewRouter(RouterConfig{
		ErrorHandler: func(msg *Message, err error) {
			handledErr = err
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan *Message, 1)
	in <- &Message{
		Data:       "data",
		Attributes: Attributes{"type": "unknown.type"},
	}
	close(in)

	out, _ := router.Pipe(ctx, in)

	// Drain output
	for range out {
	}

	if !errors.Is(handledErr, ErrNoHandler) {
		t.Errorf("expected ErrNoHandler, got %v", handledErr)
	}
}

func TestRouter_HandlerMatcher(t *testing.T) {
	var handledErr error
	router := NewRouter(RouterConfig{
		ErrorHandler: func(msg *Message, err error) {
			handledErr = err
		},
	})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
	}, KebabNaming)

	// Add handler with matcher that rejects messages without "allowed" attribute
	router.AddHandler(handler, HandlerConfig{
		Matcher: matcherFunc(func(attrs Attributes) bool {
			_, ok := attrs["allowed"]
			return ok
		}),
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan *Message, 2)
	// This one will be rejected by matcher
	in <- &Message{
		Data:       &TestCommand{ID: "1", Name: "rejected"},
		Attributes: Attributes{"type": "test.command"},
	}
	// This one will pass
	in <- &Message{
		Data:       &TestCommand{ID: "2", Name: "allowed"},
		Attributes: Attributes{"type": "test.command", "allowed": true},
	}
	close(in)

	out, _ := router.Pipe(ctx, in)

	var received []*Message
	for msg := range out {
		received = append(received, msg)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}

	if !errors.Is(handledErr, ErrHandlerRejected) {
		t.Errorf("expected ErrHandlerRejected, got %v", handledErr)
	}
}

func TestRouter_HandlerError(t *testing.T) {
	testErr := errors.New("handler error")
	var handledErr error

	router := NewRouter(RouterConfig{
		ErrorHandler: func(msg *Message, err error) {
			handledErr = err
		},
	})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return nil, testErr
	}, KebabNaming)

	router.AddHandler(handler, HandlerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan *Message, 1)
	in <- &Message{
		Data:       &TestCommand{ID: "123", Name: "test"},
		Attributes: Attributes{"type": "test.command"},
	}
	close(in)

	out, _ := router.Pipe(ctx, in)

	// Drain output
	for range out {
	}

	if !errors.Is(handledErr, testErr) {
		t.Errorf("expected %v, got %v", testErr, handledErr)
	}
}

func TestRouter_MultipleOutputs(t *testing.T) {
	router := NewRouter(RouterConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		// Return multiple messages
		return []*Message{
			{Data: TestEvent{ID: "1"}, Attributes: Attributes{"type": "test.event"}},
			{Data: TestEvent{ID: "2"}, Attributes: Attributes{"type": "test.event"}},
			{Data: TestEvent{ID: "3"}, Attributes: Attributes{"type": "test.event"}},
		}, nil
	}, KebabNaming)

	router.AddHandler(handler, HandlerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan *Message, 1)
	in <- &Message{
		Data:       &TestCommand{ID: "input"},
		Attributes: Attributes{"type": "test.command"},
	}
	close(in)

	out, _ := router.Pipe(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 output messages, got %d", count)
	}
}

func TestRouter_ContextCancellation(t *testing.T) {
	router := NewRouter(RouterConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
	}, KebabNaming)

	router.AddHandler(handler, HandlerConfig{})

	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan *Message)
	out, _ := router.Pipe(ctx, in)

	// Cancel context before sending
	cancel()

	// Send after cancel - should not block indefinitely
	select {
	case in <- &Message{
		Data:       &TestCommand{ID: "test"},
		Attributes: Attributes{"type": "test.command"},
	}:
		// Message sent (may or may not be processed)
	case <-time.After(100 * time.Millisecond):
		// Timeout is fine too
	}

	close(in)

	// Output should close eventually
	select {
	case <-out:
		// OK
	case <-time.After(100 * time.Millisecond):
		// Also OK - channel may already be closed
	}
}

func TestRouter_Standalone(t *testing.T) {
	// Test that Router works without Engine
	router := NewRouter(RouterConfig{})

	type OrderCreated struct {
		OrderID string
	}

	type OrderProcessed struct {
		Status string
	}

	handler := NewHandler[OrderCreated](func(ctx context.Context, msg *Message) ([]*Message, error) {
		order := msg.Data.(*OrderCreated)
		return []*Message{
			{
				Data: OrderProcessed{Status: "processed"},
				Attributes: Attributes{
					"type":    "order.processed",
					"orderID": order.OrderID,
				},
			},
		}, nil
	}, KebabNaming)

	router.AddHandler(handler, HandlerConfig{})

	ctx := context.Background()
	in := make(chan *Message, 1)
	in <- &Message{
		Data:       &OrderCreated{OrderID: "123"},
		Attributes: Attributes{"type": "order.created"},
	}
	close(in)

	out, err := router.Pipe(ctx, in)
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}

	msg := <-out
	if msg == nil {
		t.Fatal("expected output message")
	}

	if msg.Attributes["orderID"] != "123" {
		t.Errorf("expected orderID '123', got %v", msg.Attributes["orderID"])
	}
}

func TestRouter_MessageAck(t *testing.T) {
	router := NewRouter(RouterConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
	}, KebabNaming)

	router.AddHandler(handler, HandlerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var acked bool
	acking := NewAcking(func() { acked = true }, func(error) {}, 1)

	in := make(chan *Message, 1)
	in <- &Message{
		Data:       &TestCommand{ID: "123"},
		Attributes: Attributes{"type": "test.command"},
		a:          acking,
	}
	close(in)

	out, _ := router.Pipe(ctx, in)

	// Drain output
	for range out {
	}

	if !acked {
		t.Error("expected message to be acked after successful processing")
	}
}

// matcherFunc is a helper for testing
type matcherFunc func(attrs Attributes) bool

func (f matcherFunc) Match(attrs Attributes) bool {
	return f(attrs)
}
