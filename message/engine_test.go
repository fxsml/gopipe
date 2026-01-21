package message

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestEngine_BasicFlow(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	// Register handler
	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{
			Source: "/test",
			Naming: KebabNaming,
		},
	)
	_ = engine.AddHandler("test-handler", nil, handler)

	// Setup channels (raw I/O for broker integration)
	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("test-input", nil, input)
	output, _ := engine.AddRawOutput("test-output", nil)

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Send message
	data, _ := json.Marshal(TestCommand{ID: "123", Name: "test"})
	input <- &RawMessage{
		Data: data,
		Attributes: Attributes{
			"type": "test.command",
		},
	}

	// Receive output
	select {
	case out := <-output:
		var event TestEvent
		if err := json.Unmarshal(out.Data, &event); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}
		if event.ID != "123" || event.Status != "done" {
			t.Errorf("unexpected event: %+v", event)
		}
		if out.Type() != "test.event" {
			t.Errorf("expected type 'test.event', got %v", out.Type())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for output")
	}

	// Shutdown - close input first for natural completion
	close(input)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for engine to stop")
	}
}

func TestEngine_AlreadyStarted(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	_, err = engine.Start(ctx)
	if err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
}

func TestEngine_NoHandler(t *testing.T) {
	var lastErr error
	var mu sync.Mutex
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
		ErrorHandler: func(msg *Message, err error) {
			mu.Lock()
			lastErr = err
			mu.Unlock()
		},
	})

	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("", nil, input)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	input <- &RawMessage{
		Data:       []byte(`{}`),
		Attributes: Attributes{"type": "unknown.type"},
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if lastErr != ErrUnknownType {
		t.Errorf("expected ErrUnknownType, got %v", lastErr)
	}
}

func TestEngine_InputMatcher(t *testing.T) {
	var processedCount int
	var rejectedCount int
	var mu sync.Mutex

	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
		ErrorHandler: func(msg *Message, err error) {
			mu.Lock()
			if err == ErrInputRejected {
				rejectedCount++
			}
			mu.Unlock()
		},
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			processedCount++
			mu.Unlock()
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	// Input matcher that only accepts messages from /allowed source
	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("", &sourceMatcher{allowed: "/allowed"}, input)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "1"})

	// Send accepted message
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command", "source": "/allowed"},
	}

	// Send rejected message
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command", "source": "/rejected"},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if processedCount != 1 {
		t.Errorf("expected 1 processed, got %d", processedCount)
	}
	if rejectedCount != 1 {
		t.Errorf("expected 1 rejected, got %d", rejectedCount)
	}
	mu.Unlock()
}

type sourceMatcher struct {
	allowed string
}

func (m *sourceMatcher) Match(attrs Attributes) bool {
	source, _ := attrs["source"].(string)
	return source == m.allowed
}

func TestEngine_OutputMatcher(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("", nil, input)

	// Create two outputs with different matchers
	output1, _ := engine.AddRawOutput("", &typeMatcher{pattern: "test.event"})
	output2, _ := engine.AddRawOutput("", &typeMatcher{pattern: "other.event"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "1"})
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	select {
	case <-output1:
		// Expected
	case <-output2:
		t.Error("message should not go to output2")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

type typeMatcher struct {
	pattern string
}

func (m *typeMatcher) Match(attrs Attributes) bool {
	t, _ := attrs["type"].(string)
	return t == m.pattern
}

func TestEngine_MultipleInputs(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	var count int
	var mu sync.Mutex

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			count++
			mu.Unlock()
			return nil, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input1 := make(chan *RawMessage, 1)
	input2 := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("input1", nil, input1)
	_, _ = engine.AddRawInput("input2", nil, input2)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "1"})

	input1 <- &RawMessage{Data: data, Attributes: Attributes{"type": "test.command"}}
	input2 <- &RawMessage{Data: data, Attributes: Attributes{"type": "test.command"}}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if count != 2 {
		t.Errorf("expected 2 messages processed, got %d", count)
	}
	mu.Unlock()
}

func TestEngine_Loopback(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	// First handler creates intermediate event
	handler1 := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]IntermediateEvent, error) {
			return []IntermediateEvent{{ID: cmd.ID, Step: 1}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("handler1", nil, handler1)

	// Second handler processes intermediate event
	handler2 := NewCommandHandler(
		func(ctx context.Context, cmd IntermediateEvent) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "final"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("handler2", nil, handler2)

	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("", nil, input)

	// Loopback intermediate events (using AddOutput + AddInput pattern)
	loopbackOut, _ := engine.AddOutput("", &typeMatcher{pattern: "intermediate.event"})
	_, _ = engine.AddInput("", nil, loopbackOut)

	output, _ := engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "123"})
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	select {
	case out := <-output:
		if out.Type() != "test.event" {
			t.Errorf("expected final event type, got %v", out.Type())
		}
		var event TestEvent
		_ = json.Unmarshal(out.Data, &event)
		if event.Status != "final" {
			t.Errorf("expected status 'final', got %s", event.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for loopback result")
	}
}

type IntermediateEvent struct {
	ID   string `json:"id"`
	Step int    `json:"step"`
}

func TestEngine_HandlerError(t *testing.T) {
	var lastErr error
	var mu sync.Mutex
	testErr := errors.New("handler failed")

	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
		ErrorHandler: func(msg *Message, err error) {
			mu.Lock()
			lastErr = err
			mu.Unlock()
		},
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return nil, testErr
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("", nil, input)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "1"})
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if lastErr != testErr {
		t.Errorf("expected handler error, got %v", lastErr)
	}
}

func TestEngine_MessageAck(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("", nil, input)
	output, _ := engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	acked := false
	data, _ := json.Marshal(TestCommand{ID: "1"})
	raw := NewRaw(data, Attributes{"type": "test.command"},
		NewAcking(func() { acked = true }, func(error) {}),
	)
	input <- raw

	<-output

	time.Sleep(50 * time.Millisecond)

	if !acked {
		t.Error("expected message to be acked after processing")
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 100 * time.Millisecond, // Force shutdown after timeout
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage)
	_, _ = engine.AddRawInput("", nil, input)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())

	done, _ := engine.Start(ctx)

	// Cancel without closing input - relies on ShutdownTimeout to force
	cancel()

	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("engine did not stop after context cancellation")
	}
}

func TestEngine_DrainsInFlightMessages(t *testing.T) {
	// This test verifies that messages which pass the merger are delivered
	// to outputs even after context cancellation triggers forced shutdown.
	// The merger has ShutdownTimeout, but distributor waits indefinitely
	// for its input to close, ensuring cascading drain.

	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 50 * time.Millisecond,
	})

	var processed []string
	var mu sync.Mutex

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			processed = append(processed, cmd.ID)
			mu.Unlock()
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("", nil, input)
	output, _ := engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())

	done, _ := engine.Start(ctx)

	// Send messages before cancellation
	for i := 0; i < 3; i++ {
		data, _ := json.Marshal(TestCommand{ID: string(rune('A' + i)), Name: "test"})
		input <- &RawMessage{
			Data:       data,
			Attributes: Attributes{"type": "test.command"},
		}
	}

	// Small delay to let messages enter the pipeline
	time.Sleep(10 * time.Millisecond)

	// Cancel context - merger will force shutdown after 50ms
	// but messages already in router/distributor should be delivered
	cancel()

	// Collect all outputs before done closes
	var received []string
	collectDone := make(chan struct{})
	go func() {
		defer close(collectDone)
		for out := range output {
			var event TestEvent
			if err := json.Unmarshal(out.Data, &event); err == nil {
				received = append(received, event.ID)
			}
		}
	}()

	// Wait for engine to stop
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("engine did not stop")
	}

	// Wait for output collection to finish
	select {
	case <-collectDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout collecting outputs")
	}

	// Verify messages that entered the pipeline were delivered
	mu.Lock()
	processedCount := len(processed)
	mu.Unlock()

	if processedCount == 0 {
		t.Fatal("no messages were processed")
	}

	// All processed messages should have been output (drain guarantee)
	if len(received) != processedCount {
		t.Errorf("drain guarantee violated: processed %d but received %d outputs",
			processedCount, len(received))
	}
}

func TestEngine_AddInputAfterStart(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	var count int
	var mu sync.Mutex

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			count++
			mu.Unlock()
			return nil, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	// Add initial input before start
	input1 := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("input1", nil, input1)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Add second input after start
	input2 := make(chan *RawMessage, 1)
	_, err = engine.AddRawInput("input2", nil, input2)
	if err != nil {
		t.Fatalf("failed to add input after start: %v", err)
	}

	data, _ := json.Marshal(TestCommand{ID: "1"})

	// Send to first input
	input1 <- &RawMessage{Data: data, Attributes: Attributes{"type": "test.command"}}

	// Send to second input (added after start)
	input2 <- &RawMessage{Data: data, Attributes: Attributes{"type": "test.command"}}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if count != 2 {
		t.Errorf("expected 2 messages processed (one from each input), got %d", count)
	}
	mu.Unlock()
}

func TestEngine_AddOutputAfterStart(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage, 2)
	_, _ = engine.AddRawInput("", nil, input)

	// Add first output before start - matches only "other.event"
	output1, _ := engine.AddRawOutput("", &typeMatcher{pattern: "other.event"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Add second output after start - matches "test.event"
	// This output added after Start should receive the message
	output2, _ := engine.AddRawOutput("", &typeMatcher{pattern: "test.event"})

	data, _ := json.Marshal(TestCommand{ID: "1"})

	// Send message that produces test.event
	input <- &RawMessage{Data: data, Attributes: Attributes{"type": "test.command"}}

	// output2 (added after start) should receive the message
	// since output1 doesn't match "test.event"
	select {
	case <-output2:
		// Expected - output added after start works
	case <-output1:
		t.Fatal("output1 should not receive test.event")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for output2 (added after start)")
	}
}

// TestEngine_TypedIO tests the typed input/output API (AddInput/AddOutput with *Message).
func TestEngine_TypedIO(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	// Use typed input/output (no marshal/unmarshal)
	input := make(chan *Message, 1)
	_, _ = engine.AddInput("typed-input", nil, input)
	output, _ := engine.AddOutput("typed-output", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Send typed message (already unmarshaled)
	input <- &Message{
		Data:       &TestCommand{ID: "123", Name: "test"},
		Attributes: Attributes{"type": "test.command"},
	}

	// Receive typed output (not marshaled)
	select {
	case out := <-output:
		// Handler returns []TestEvent (values), so we receive TestEvent, not *TestEvent
		event, ok := out.Data.(TestEvent)
		if !ok {
			t.Fatalf("expected TestEvent, got %T", out.Data)
		}
		if event.ID != "123" || event.Status != "done" {
			t.Errorf("unexpected event: %+v", event)
		}
		if out.Type() != "test.event" {
			t.Errorf("expected type 'test.event', got %v", out.Type())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for typed output")
	}

	// Shutdown - close input first for natural completion
	close(input)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for engine to stop")
	}
}

// TestEngine_MixedIO tests mixing raw and typed inputs/outputs.
func TestEngine_MixedIO(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
	})

	var count int
	var mu sync.Mutex

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			count++
			mu.Unlock()
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	// Mix raw and typed inputs
	rawInput := make(chan *RawMessage, 1)
	typedInput := make(chan *Message, 1)
	_, _ = engine.AddRawInput("raw-input", nil, rawInput)
	_, _ = engine.AddInput("typed-input", nil, typedInput)

	// Mix raw and typed outputs
	rawOutput, _ := engine.AddRawOutput("", &typeMatcher{pattern: "test.event"})
	typedOutput, _ := engine.AddOutput("", &typeMatcher{pattern: "test.event"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	// Send raw message
	data, _ := json.Marshal(TestCommand{ID: "1"})
	rawInput <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	// Send typed message
	typedInput <- &Message{
		Data:       &TestCommand{ID: "2", Name: "test"},
		Attributes: Attributes{"type": "test.command"},
	}

	// Wait for both outputs
	received := 0
	timeout := time.After(time.Second)
	for received < 2 {
		select {
		case <-rawOutput:
			received++
		case <-typedOutput:
			received++
		case <-timeout:
			t.Fatalf("timeout: only received %d of 2 messages", received)
		}
	}

	mu.Lock()
	if count != 2 {
		t.Errorf("expected 2 messages processed, got %d", count)
	}
	mu.Unlock()
}

// TestEngine_HandlerMatcher tests that handler matcher is applied after type matching.
func TestEngine_HandlerMatcher(t *testing.T) {
	var processedCount int
	var rejectedCount int
	var mu sync.Mutex

	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
		ErrorHandler: func(msg *Message, err error) {
			mu.Lock()
			if err == ErrHandlerRejected {
				rejectedCount++
			}
			mu.Unlock()
		},
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			processedCount++
			mu.Unlock()
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)

	// Register handler with a matcher that only accepts messages from /allowed source
	_ = engine.AddHandler("test", &sourceMatcher{allowed: "/allowed"}, handler)

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("", nil, input)
	_, _ = engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "1"})

	// Send message from allowed source - should be processed
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command", "source": "/allowed"},
	}

	// Send message from rejected source - should be rejected by handler matcher
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command", "source": "/rejected"},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if processedCount != 1 {
		t.Errorf("expected 1 processed, got %d", processedCount)
	}
	if rejectedCount != 1 {
		t.Errorf("expected 1 rejected by handler matcher, got %d", rejectedCount)
	}
	mu.Unlock()
}

func TestEngine_NoShutdownTimeout_WaitsIndefinitely(t *testing.T) {
	// With ShutdownTimeout <= 0, the engine should wait indefinitely
	// for inputs to close naturally, even after context cancellation.

	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 0, // Wait indefinitely
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage)
	_, _ = engine.AddRawInput("", nil, input)
	output, _ := engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())

	done, _ := engine.Start(ctx)

	// Cancel context - should NOT stop engine (timeout=0 means wait indefinitely)
	cancel()

	// Engine should NOT stop yet (input still open)
	select {
	case <-done:
		t.Fatal("engine stopped but should wait indefinitely for input to close")
	case <-time.After(100 * time.Millisecond):
		// Expected - still waiting
	}

	// Send a message after cancel - should still be processed
	data, _ := json.Marshal(TestCommand{ID: "1"})
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	// Should receive output
	select {
	case <-output:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("message not processed after context cancel with timeout=0")
	}

	// Now close input - engine should stop
	close(input)

	select {
	case <-done:
		// Expected - engine stopped after input closed
	case <-time.After(time.Second):
		t.Fatal("engine did not stop after input closed")
	}
}

func TestEngine_AddPlugin(t *testing.T) {
	t.Run("plugin adds handler", func(t *testing.T) {
		engine := NewEngine(EngineConfig{
			Marshaler: NewJSONMarshaler(),
		})

		// Plugin that adds a handler
		plugin := func(e *Engine) error {
			handler := NewCommandHandler(
				func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
					return []TestEvent{{ID: cmd.ID, Status: "from-plugin"}}, nil
				},
				CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
			)
			return e.AddHandler("plugin-handler", nil, handler)
		}

		err := engine.AddPlugin(plugin)
		if err != nil {
			t.Fatalf("AddPlugin failed: %v", err)
		}

		// Verify handler works
		input := make(chan *Message, 1)
		_, _ = engine.AddInput("", nil, input)
		output, _ := engine.AddOutput("", nil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, _ = engine.Start(ctx)

		input <- &Message{
			Data:       &TestCommand{ID: "123"},
			Attributes: Attributes{"type": "test.command"},
		}

		select {
		case out := <-output:
			event := out.Data.(TestEvent)
			if event.Status != "from-plugin" {
				t.Errorf("expected status 'from-plugin', got %s", event.Status)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	})

	t.Run("plugin error propagates", func(t *testing.T) {
		engine := NewEngine(EngineConfig{
			Marshaler: NewJSONMarshaler(),
		})

		expectedErr := errors.New("plugin failed")
		plugin := func(e *Engine) error {
			return expectedErr
		}

		err := engine.AddPlugin(plugin)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("multiple plugins", func(t *testing.T) {
		engine := NewEngine(EngineConfig{
			Marshaler: NewJSONMarshaler(),
		})

		var order []string
		plugin1 := func(e *Engine) error {
			order = append(order, "plugin1")
			return nil
		}
		plugin2 := func(e *Engine) error {
			order = append(order, "plugin2")
			return nil
		}

		err := engine.AddPlugin(plugin1, plugin2)
		if err != nil {
			t.Fatalf("AddPlugin failed: %v", err)
		}

		if len(order) != 2 || order[0] != "plugin1" || order[1] != "plugin2" {
			t.Errorf("expected [plugin1, plugin2], got %v", order)
		}
	})

	t.Run("plugin chain stops on error", func(t *testing.T) {
		engine := NewEngine(EngineConfig{
			Marshaler: NewJSONMarshaler(),
		})

		var order []string
		plugin1 := func(e *Engine) error {
			order = append(order, "plugin1")
			return errors.New("plugin1 failed")
		}
		plugin2 := func(e *Engine) error {
			order = append(order, "plugin2")
			return nil
		}

		err := engine.AddPlugin(plugin1, plugin2)
		if err == nil {
			t.Fatal("expected error")
		}

		if len(order) != 1 || order[0] != "plugin1" {
			t.Errorf("expected [plugin1] only, got %v", order)
		}
	})
}

func TestEngine_NoGoroutineLeakOnShutdown(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 50 * time.Millisecond,
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("", nil, input)
	output, _ := engine.AddRawOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())

	done, _ := engine.Start(ctx)

	// Send some messages
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(TestCommand{ID: string(rune('A' + i))})
		input <- &RawMessage{
			Data:       data,
			Attributes: Attributes{"type": "test.command"},
		}
	}

	// Consume some outputs
	for i := 0; i < 3; i++ {
		select {
		case <-output:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for output")
		}
	}

	// Close input first for graceful shutdown (unmarshal pipe waits for input)
	close(input)
	cancel()

	// Wait for engine to stop
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("engine did not stop")
	}

	// Drain any remaining outputs
	for range output {
	}

	// Give goroutines time to clean up
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}

func TestEngine_RouterPool(t *testing.T) {
	// Verify that RouterPool config is propagated to the router.
	// With workers=3, messages should be processed in parallel.

	const workers = 3
	const messageCount = 3
	const processTime = 50 * time.Millisecond

	engine := NewEngine(EngineConfig{
		RouterPool: PoolConfig{Workers: workers},
	})

	var activeCount int
	var maxActive int
	var mu sync.Mutex

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			mu.Lock()
			activeCount++
			if activeCount > maxActive {
				maxActive = activeCount
			}
			mu.Unlock()

			time.Sleep(processTime)

			mu.Lock()
			activeCount--
			mu.Unlock()

			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *Message, messageCount)
	_, _ = engine.AddInput("", nil, input)
	output, _ := engine.AddOutput("", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, _ := engine.Start(ctx)

	// Send all messages at once
	start := time.Now()
	for i := 0; i < messageCount; i++ {
		input <- &Message{
			Data:       &TestCommand{ID: string(rune('A' + i))},
			Attributes: Attributes{"type": "test.command"},
		}
	}

	// Collect all outputs
	for i := 0; i < messageCount; i++ {
		select {
		case <-output:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for output %d", i)
		}
	}
	elapsed := time.Since(start)

	// With concurrent processing, total time should be less than sequential
	// Sequential: messageCount * processTime = 150ms
	// Concurrent: ~processTime = ~50ms (plus overhead)
	maxExpected := time.Duration(messageCount) * processTime
	if elapsed >= maxExpected {
		t.Errorf("messages processed sequentially: elapsed=%v, expected<%v", elapsed, maxExpected)
	}

	// Verify concurrent execution actually happened
	mu.Lock()
	if maxActive < 2 {
		t.Errorf("expected concurrent execution (maxActive>=2), got maxActive=%d", maxActive)
	}
	mu.Unlock()

	close(input)
	cancel()
	<-done
}

func TestEngine_DefaultMarshaler(t *testing.T) {
	// Engine should default to JSONMarshaler when Marshaler is not set.
	// This ensures raw inputs can be processed without explicit marshaler config.
	// See: https://github.com/fxsml/gopipe/issues/85
	engine := NewEngine(EngineConfig{})

	// Register handler
	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{
			Source: "/test",
			Naming: KebabNaming,
		},
	)
	_ = engine.AddHandler("test-handler", nil, handler)

	// Setup raw I/O (this is where the panic occurred with nil marshaler)
	input := make(chan *RawMessage, 1)
	_, _ = engine.AddRawInput("test-input", nil, input)
	output, _ := engine.AddRawOutput("test-output", nil)

	// Start engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Send message - this would panic with nil marshaler
	data, _ := json.Marshal(TestCommand{ID: "456", Name: "default-marshaler-test"})
	input <- &RawMessage{
		Data: data,
		Attributes: Attributes{
			"type": "test.command",
		},
	}

	// Receive output
	select {
	case out := <-output:
		var event TestEvent
		if err := json.Unmarshal(out.Data, &event); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}
		if event.ID != "456" || event.Status != "done" {
			t.Errorf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for output")
	}

	// Shutdown
	close(input)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for engine to stop")
	}
}
