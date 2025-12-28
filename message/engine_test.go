package message

import (
	"context"
	"encoding/json"
	"errors"
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
	engine.AddHandler(handler, HandlerConfig{Name: "test-handler"})

	// Setup channels (raw I/O for broker integration)
	input := make(chan *RawMessage, 1)
	engine.AddRawInput(input, RawInputConfig{Name: "test-input"})
	output := engine.AddRawOutput(RawOutputConfig{Name: "test-output"})

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
		if out.Attributes["type"] != "test.event" {
			t.Errorf("expected type 'test.event', got %v", out.Attributes["type"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for output")
	}

	// Shutdown
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
	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
		ErrorHandler: func(msg *Message, err error) {
			lastErr = err
		},
	})

	input := make(chan *RawMessage, 1)
	engine.AddRawInput(input, RawInputConfig{})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	input <- &RawMessage{
		Data:       []byte(`{}`),
		Attributes: Attributes{"type": "unknown.type"},
	}

	time.Sleep(50 * time.Millisecond)

	if lastErr != ErrNoHandler {
		t.Errorf("expected ErrNoHandler, got %v", lastErr)
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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	// Input matcher that only accepts messages from /allowed source
	input := make(chan *RawMessage, 10)
	engine.AddRawInput(input, RawInputConfig{
		Matcher: &sourceMatcher{allowed: "/allowed"},
	})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	input := make(chan *RawMessage, 1)
	engine.AddRawInput(input, RawInputConfig{})

	// Create two outputs with different matchers
	output1 := engine.AddRawOutput(RawOutputConfig{
		Matcher: &typeMatcher{pattern: "test.event"},
	})
	output2 := engine.AddRawOutput(RawOutputConfig{
		Matcher: &typeMatcher{pattern: "other.event"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	input1 := make(chan *RawMessage, 1)
	input2 := make(chan *RawMessage, 1)
	engine.AddRawInput(input1, RawInputConfig{Name: "input1"})
	engine.AddRawInput(input2, RawInputConfig{Name: "input2"})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

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
	engine.AddHandler(handler1, HandlerConfig{Name: "handler1"})

	// Second handler processes intermediate event
	handler2 := NewCommandHandler(
		func(ctx context.Context, cmd IntermediateEvent) ([]TestEvent, error) {
			return []TestEvent{{ID: cmd.ID, Status: "final"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	engine.AddHandler(handler2, HandlerConfig{Name: "handler2"})

	input := make(chan *RawMessage, 1)
	engine.AddRawInput(input, RawInputConfig{})

	// Loopback intermediate events
	engine.AddLoopback(LoopbackConfig{
		Matcher: &typeMatcher{pattern: "intermediate.event"},
	})

	output := engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "123"})
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	select {
	case out := <-output:
		if out.Attributes["type"] != "test.event" {
			t.Errorf("expected final event type, got %v", out.Attributes["type"])
		}
		var event TestEvent
		json.Unmarshal(out.Data, &event)
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
	testErr := errors.New("handler failed")

	engine := NewEngine(EngineConfig{
		Marshaler: NewJSONMarshaler(),
		ErrorHandler: func(msg *Message, err error) {
			lastErr = err
		},
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return nil, testErr
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	input := make(chan *RawMessage, 1)
	engine.AddRawInput(input, RawInputConfig{})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	data, _ := json.Marshal(TestCommand{ID: "1"})
	input <- &RawMessage{
		Data:       data,
		Attributes: Attributes{"type": "test.command"},
	}

	time.Sleep(50 * time.Millisecond)

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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	input := make(chan *RawMessage, 1)
	engine.AddRawInput(input, RawInputConfig{})
	output := engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	acked := false
	data, _ := json.Marshal(TestCommand{ID: "1"})
	raw := NewWithAcking(data, Attributes{"type": "test.command"},
		func() { acked = true },
		func(error) {},
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
		Marshaler: NewJSONMarshaler(),
	})

	handler := NewCommandHandler(
		func(ctx context.Context, cmd TestCommand) ([]TestEvent, error) {
			return []TestEvent{}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	)
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	input := make(chan *RawMessage)
	engine.AddRawInput(input, RawInputConfig{})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())

	done, _ := engine.Start(ctx)

	cancel()

	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("engine did not stop after context cancellation")
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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	// Add initial input before start
	input1 := make(chan *RawMessage, 1)
	engine.AddRawInput(input1, RawInputConfig{Name: "input1"})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Add second input after start
	input2 := make(chan *RawMessage, 1)
	err = engine.AddRawInput(input2, RawInputConfig{Name: "input2"})
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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	input := make(chan *RawMessage, 2)
	engine.AddRawInput(input, RawInputConfig{})

	// Add first output before start - matches only "other.event"
	output1 := engine.AddRawOutput(RawOutputConfig{
		Matcher: &typeMatcher{pattern: "other.event"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Add second output after start - matches "test.event"
	// This output added after Start should receive the message
	output2 := engine.AddRawOutput(RawOutputConfig{
		Matcher: &typeMatcher{pattern: "test.event"},
	})

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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	// Use typed input/output (no marshal/unmarshal)
	input := make(chan *Message, 1)
	engine.AddInput(input, InputConfig{Name: "typed-input"})
	output := engine.AddOutput(OutputConfig{Name: "typed-output"})

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
		if out.Attributes["type"] != "test.event" {
			t.Errorf("expected type 'test.event', got %v", out.Attributes["type"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for typed output")
	}

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
	engine.AddHandler(handler, HandlerConfig{Name: "test"})

	// Mix raw and typed inputs
	rawInput := make(chan *RawMessage, 1)
	typedInput := make(chan *Message, 1)
	engine.AddRawInput(rawInput, RawInputConfig{Name: "raw-input"})
	engine.AddInput(typedInput, InputConfig{Name: "typed-input"})

	// Mix raw and typed outputs
	rawOutput := engine.AddRawOutput(RawOutputConfig{
		Matcher: &typeMatcher{pattern: "test.event"},
	})
	typedOutput := engine.AddOutput(OutputConfig{
		Matcher: &typeMatcher{pattern: "test.event"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

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
	engine.AddHandler(handler, HandlerConfig{
		Name:    "test",
		Matcher: &sourceMatcher{allowed: "/allowed"},
	})

	input := make(chan *RawMessage, 10)
	engine.AddRawInput(input, RawInputConfig{})
	engine.AddRawOutput(RawOutputConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

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
