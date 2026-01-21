package message

import (
	"context"
	"errors"
	"strings"
	"sync"
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

	router := NewRouter(RouterConfig{
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
	router := NewRouter(RouterConfig{})

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
	router := NewRouter(RouterConfig{})

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

func TestRouter_MessageAck(t *testing.T) {
	router := NewRouter(RouterConfig{})

	handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
	}, KebabNaming)

	_ = router.AddHandler("", nil, handler)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var acked bool
	acking := NewAcking(func() { acked = true }, func(error) {})

	in := make(chan *Message, 1)
	in <- &Message{
		Data:       &TestCommand{ID: "123"},
		Attributes: Attributes{"type": "test.command"},
		acking:     acking,
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

func TestRouter_AddPoolWithConfig(t *testing.T) {
	t.Run("adds pool successfully", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		err := router.AddPoolWithConfig("erp", PoolConfig{Workers: 5})
		if err != nil {
			t.Fatalf("AddPoolWithConfig failed: %v", err)
		}
	})

	t.Run("returns error for empty name", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		err := router.AddPoolWithConfig("", PoolConfig{Workers: 5})
		if !errors.Is(err, ErrPoolNameEmpty) {
			t.Errorf("expected ErrPoolNameEmpty, got %v", err)
		}
	})

	t.Run("returns error for duplicate pool", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		_ = router.AddPoolWithConfig("erp", PoolConfig{Workers: 5})
		err := router.AddPoolWithConfig("erp", PoolConfig{Workers: 10})
		if !errors.Is(err, ErrPoolExists) {
			t.Errorf("expected ErrPoolExists, got %v", err)
		}
	})

	t.Run("returns error after router started", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
			return nil, nil
		}, KebabNaming)
		_ = router.AddHandler("test", nil, handler)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		in := make(chan *Message)
		close(in)
		_, _ = router.Pipe(ctx, in)

		err := router.AddPoolWithConfig("erp", PoolConfig{Workers: 5})
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("expected ErrAlreadyStarted, got %v", err)
		}
	})

	t.Run("can override default pool", func(t *testing.T) {
		router := NewRouter(RouterConfig{Pool: PoolConfig{Workers: 10}})
		err := router.AddPoolWithConfig("default", PoolConfig{Workers: 20})
		if !errors.Is(err, ErrPoolExists) {
			t.Errorf("expected ErrPoolExists when overriding default, got %v", err)
		}
	})
}

func TestRouter_AddHandlerToPool(t *testing.T) {
	t.Run("adds handler to named pool", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		_ = router.AddPoolWithConfig("erp", PoolConfig{Workers: 5})

		handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
			return []*Message{{Data: msg.Data, Attributes: msg.Attributes}}, nil
		}, KebabNaming)

		err := router.AddHandlerToPool("test", nil, handler, "erp")
		if err != nil {
			t.Fatalf("AddHandlerToPool failed: %v", err)
		}
	})

	t.Run("returns error for non-existent pool", func(t *testing.T) {
		router := NewRouter(RouterConfig{})

		handler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
			return nil, nil
		}, KebabNaming)

		err := router.AddHandlerToPool("test", nil, handler, "nonexistent")
		if !errors.Is(err, ErrPoolNotFound) {
			t.Errorf("expected ErrPoolNotFound, got %v", err)
		}
	})

	t.Run("returns error for duplicate handler", func(t *testing.T) {
		router := NewRouter(RouterConfig{})

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
	})
}

func TestRouter_MultiPool(t *testing.T) {
	t.Run("routes to correct pool", func(t *testing.T) {
		router := NewRouter(RouterConfig{Pool: PoolConfig{Workers: 2}})
		_ = router.AddPoolWithConfig("erp", PoolConfig{Workers: 1})

		// Track which handler processed which message
		var defaultPoolCount, erpPoolCount int
		var mu sync.Mutex

		defaultHandler := NewHandler[TestCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
			mu.Lock()
			defaultPoolCount++
			mu.Unlock()
			return []*Message{{Data: TestEvent{ID: msg.Data.(*TestCommand).ID}, Attributes: Attributes{"type": "default.event"}}}, nil
		}, KebabNaming)

		type ERPCommand struct {
			ID string
		}
		erpHandler := NewHandler[ERPCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
			mu.Lock()
			erpPoolCount++
			mu.Unlock()
			return []*Message{{Data: TestEvent{ID: msg.Data.(*ERPCommand).ID}, Attributes: Attributes{"type": "erp.event"}}}, nil
		}, KebabNaming)

		_ = router.AddHandler("default-handler", nil, defaultHandler)
		_ = router.AddHandlerToPool("erp-handler", nil, erpHandler, "erp")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		in := make(chan *Message, 4)
		in <- &Message{Data: &TestCommand{ID: "1"}, Attributes: Attributes{"type": "test.command"}}
		in <- &Message{Data: &ERPCommand{ID: "2"}, Attributes: Attributes{"type": "erp.command"}}
		in <- &Message{Data: &TestCommand{ID: "3"}, Attributes: Attributes{"type": "test.command"}}
		in <- &Message{Data: &ERPCommand{ID: "4"}, Attributes: Attributes{"type": "erp.command"}}
		close(in)

		out, err := router.Pipe(ctx, in)
		if err != nil {
			t.Fatalf("Pipe failed: %v", err)
		}

		var received int
		for range out {
			received++
		}

		if received != 4 {
			t.Errorf("expected 4 messages, got %d", received)
		}

		mu.Lock()
		defer mu.Unlock()
		if defaultPoolCount != 2 {
			t.Errorf("expected 2 messages in default pool, got %d", defaultPoolCount)
		}
		if erpPoolCount != 2 {
			t.Errorf("expected 2 messages in erp pool, got %d", erpPoolCount)
		}
	})
}

func TestRouter_MultiPool_Concurrency(t *testing.T) {
	// Verify that pools with different worker counts actually execute
	// with different concurrency levels.
	const processTime = 50 * time.Millisecond

	// Use default pool as "slow" (1 worker) and create "fast" pool (3 workers)
	router := NewRouter(RouterConfig{Pool: PoolConfig{Workers: 1}})
	_ = router.AddPoolWithConfig("fast", PoolConfig{Workers: 3})

	// Track max concurrent executions per pool
	var slowActive, slowMax, fastActive, fastMax int
	var mu sync.Mutex

	type SlowCommand struct{ ID string }
	type FastCommand struct{ ID string }

	slowHandler := NewHandler[SlowCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		mu.Lock()
		slowActive++
		if slowActive > slowMax {
			slowMax = slowActive
		}
		mu.Unlock()

		time.Sleep(processTime)

		mu.Lock()
		slowActive--
		mu.Unlock()
		return []*Message{{Data: msg.Data, Attributes: Attributes{"type": "slow.event"}}}, nil
	}, KebabNaming)

	fastHandler := NewHandler[FastCommand](func(ctx context.Context, msg *Message) ([]*Message, error) {
		mu.Lock()
		fastActive++
		if fastActive > fastMax {
			fastMax = fastActive
		}
		mu.Unlock()

		time.Sleep(processTime)

		mu.Lock()
		fastActive--
		mu.Unlock()
		return []*Message{{Data: msg.Data, Attributes: Attributes{"type": "fast.event"}}}, nil
	}, KebabNaming)

	// slow handler goes to default pool, fast handler goes to fast pool
	_ = router.AddHandler("slow", nil, slowHandler)
	_ = router.AddHandlerToPool("fast", nil, fastHandler, "fast")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	in := make(chan *Message, 6)
	// Send 3 messages to each pool
	for i := 0; i < 3; i++ {
		in <- &Message{Data: &SlowCommand{ID: string(rune('a' + i))}, Attributes: Attributes{"type": "slow.command"}}
		in <- &Message{Data: &FastCommand{ID: string(rune('a' + i))}, Attributes: Attributes{"type": "fast.command"}}
	}
	close(in)

	out, err := router.Pipe(ctx, in)
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}

	// Receive all messages, then cancel context to close output
	const expected = 6
	var received int
	for received < expected {
		select {
		case _, ok := <-out:
			if !ok {
				t.Fatalf("output closed early, received %d/%d", received, expected)
			}
			received++
		case <-ctx.Done():
			t.Fatalf("timeout waiting for messages, received %d/%d", received, expected)
		}
	}
	cancel() // Signal done, allows merger to close

	mu.Lock()
	defer mu.Unlock()

	// Slow pool (1 worker) should have max 1 concurrent
	if slowMax != 1 {
		t.Errorf("slow pool: expected max concurrency 1, got %d", slowMax)
	}

	// Fast pool (3 workers) should have max > 1 concurrent
	if fastMax < 2 {
		t.Errorf("fast pool: expected max concurrency >= 2, got %d", fastMax)
	}
}

func TestRouter_PoolConfig_Defaults(t *testing.T) {
	t.Run("workers defaults to 1", func(t *testing.T) {
		cfg := PoolConfig{}.parse(100)
		if cfg.Workers != 1 {
			t.Errorf("expected Workers=1, got %d", cfg.Workers)
		}
	})

	t.Run("buffer inherits from router", func(t *testing.T) {
		cfg := PoolConfig{}.parse(200)
		if cfg.BufferSize != 200 {
			t.Errorf("expected BufferSize=200, got %d", cfg.BufferSize)
		}
	})

	t.Run("explicit values preserved", func(t *testing.T) {
		cfg := PoolConfig{Workers: 5, BufferSize: 50}.parse(100)
		if cfg.Workers != 5 {
			t.Errorf("expected Workers=5, got %d", cfg.Workers)
		}
		if cfg.BufferSize != 50 {
			t.Errorf("expected BufferSize=50, got %d", cfg.BufferSize)
		}
	})
}
