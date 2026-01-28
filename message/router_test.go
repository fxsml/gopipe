package message

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRouter_BasicRouting(t *testing.T) {
	router := NewRouter(PipeConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		cmd := msg.Data.(*TestCommand)
		return []*Message{
			{
				Data:       TestEvent{ID: cmd.ID, Status: "processed"},
				Attributes: msg.Attributes,
			},
		}, nil
	}, KebabNaming)

	_ = router.AddHandler("", nil, handler)

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
	router := NewRouter(PipeConfig{
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
	router := NewRouter(PipeConfig{
		ErrorHandler: func(msg *Message, err error) {
			handledErr = err
		},
	})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
	}, KebabNaming)

	// Add handler with matcher that rejects messages without "allowed" attribute
	_ = router.AddHandler("", matcherFunc(func(attrs Attributes) bool {
		_, ok := attrs["allowed"]
		return ok
	}), handler)

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

	router := NewRouter(PipeConfig{
		ErrorHandler: func(msg *Message, err error) {
			handledErr = err
		},
	})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return nil, testErr
	}, KebabNaming)

	_ = router.AddHandler("", nil, handler)

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
	router := NewRouter(PipeConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		// Return multiple messages
		return []*Message{
			{Data: TestEvent{ID: "1"}, Attributes: Attributes{"type": "test.event"}},
			{Data: TestEvent{ID: "2"}, Attributes: Attributes{"type": "test.event"}},
			{Data: TestEvent{ID: "3"}, Attributes: Attributes{"type": "test.event"}},
		}, nil
	}, KebabNaming)

	_ = router.AddHandler("", nil, handler)

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
	router := NewRouter(PipeConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
	}, KebabNaming)

	_ = router.AddHandler("", nil, handler)

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
	router := NewRouter(PipeConfig{})

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

	_ = router.AddHandler("", nil, handler)

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

// matcherFunc is a helper for testing
type matcherFunc func(attrs Attributes) bool

func (f matcherFunc) Match(attrs Attributes) bool {
	return f(attrs)
}

func TestFuncName(t *testing.T) {
	// Factory-returned function shows factory name
	factory := testMiddlewareFactory()
	if got := funcName(factory); got != "testMiddlewareFactory" {
		t.Errorf("funcName: got %q, want %q", got, "testMiddlewareFactory")
	}
}

func TestFuncName_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		fn       any
		wantName string
	}{
		{
			name:     "non-generic factory function",
			fn:       testMiddlewareFactory(),
			wantName: "testMiddlewareFactory",
		},
		{
			name:     "generic factory function with single type param",
			fn:       testGenericFactory[string](),
			wantName: "testGenericFactory",
		},
		{
			name:     "generic factory function with int type",
			fn:       testGenericFactory[int](),
			wantName: "testGenericFactory",
		},
		{
			name:     "generic factory with multiple type params",
			fn:       testGenericFactoryMulti[string, int](),
			wantName: "testGenericFactoryMulti",
		},
		{
			name:     "generic factory with nested generics",
			fn:       testGenericFactory[map[string][]int](),
			wantName: "testGenericFactory",
		},
		{
			name:     "generic factory with struct type param",
			fn:       testGenericFactory[Attributes](),
			wantName: "testGenericFactory",
		},
		{
			name:     "generic factory with pointer type param",
			fn:       testGenericFactory[*Message](),
			wantName: "testGenericFactory",
		},
		{
			name:     "package-level function includes package prefix",
			fn:       testStandaloneFunc,
			wantName: "message.testStandaloneFunc",
		},
		{
			name:     "function from external package",
			fn:       context.Background,
			wantName: "context.Background",
		},
		{
			name:     "generic package-level function (not closure)",
			fn:       testGenericFunc[string],
			wantName: "message.testGenericFunc",
		},
		{
			name:     "method expression",
			fn:       (*strings.Builder).WriteString,
			wantName: "strings.(*Builder).WriteString",
		},
		{
			name:     "function named funcXxx not misidentified as closure",
			fn:       funcHelper,
			wantName: "message.funcHelper",
		},
		{
			name:     "nested closure traverses to factory",
			fn:       outerFactory()(),
			wantName: "outerFactory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := funcName(tt.fn)
			if got != tt.wantName {
				t.Errorf("funcName() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

// testGenericFactory returns a closure from a generic function.
// This is the pattern that caused issue #79 - funcName returned "]"
// because the runtime name is like "pkg.testGenericFactory[string].func1"
func testGenericFactory[T any]() Middleware {
	return func(next ProcessFunc) ProcessFunc { return next }
}

// testGenericFactoryMulti tests multiple type parameters.
// Runtime name is like "pkg.testGenericFactoryMulti[string,int].func1"
func testGenericFactoryMulti[K comparable, V any]() Middleware {
	return func(next ProcessFunc) ProcessFunc { return next }
}

// testStandaloneFunc is a package-level function, not a closure.
// funcName returns package.FunctionName.
func testStandaloneFunc(next ProcessFunc) ProcessFunc { return next }

// testGenericFunc is a generic package-level function (not returning a closure).
// Tests that generic type params are stripped: "pkg.testGenericFunc[string]" -> "message.testGenericFunc"
func testGenericFunc[T any]() T { var zero T; return zero }

// funcHelper is a function whose name starts with "func".
// Should NOT be misidentified as a closure (func1, func2, etc.).
func funcHelper() {}

// outerFactory returns a nested closure to test nested closure handling.
func outerFactory() func() func() {
	return func() func() {
		return func() {}
	}
}

func testMiddlewareFactory() Middleware {
	return func(next ProcessFunc) ProcessFunc { return next }
}

// Pool tests

func TestRouter_DuplicateHandler(t *testing.T) {
	router := NewRouter(PipeConfig{})

	handler1 := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return nil, nil
	}, KebabNaming)
	handler2 := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return nil, nil
	}, KebabNaming)

	_ = router.AddHandler("test1", nil, handler1)
	err := router.AddHandler("test2", nil, handler2)
	if !errors.Is(err, ErrHandlerExists) {
		t.Errorf("expected ErrHandlerExists, got %v", err)
	}
}

func TestRouter_PoolConfig_Defaults(t *testing.T) {
	t.Run("workers defaults to 1", func(t *testing.T) {
		cfg := PoolConfig{}.parse()
		if cfg.Workers != 1 {
			t.Errorf("expected Workers=1, got %d", cfg.Workers)
		}
	})

	t.Run("buffer defaults to 100", func(t *testing.T) {
		cfg := PoolConfig{}.parse()
		if cfg.BufferSize != 100 {
			t.Errorf("expected BufferSize=100, got %d", cfg.BufferSize)
		}
	})

	t.Run("explicit values preserved", func(t *testing.T) {
		cfg := PoolConfig{Workers: 5, BufferSize: 50}.parse()
		if cfg.Workers != 5 {
			t.Errorf("expected Workers=5, got %d", cfg.Workers)
		}
		if cfg.BufferSize != 50 {
			t.Errorf("expected BufferSize=50, got %d", cfg.BufferSize)
		}
	})
}
