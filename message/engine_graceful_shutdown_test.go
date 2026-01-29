package message

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

// Test types for graceful shutdown tests
type Step1Command struct {
	ID string `json:"id"`
}

type FinalEvent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// shutdownTypeMatcher for tests
type shutdownTypeMatcher struct {
	pattern string
}

func (m *shutdownTypeMatcher) Match(attrs Attributes) bool {
	t, _ := attrs["type"].(string)
	return t == m.pattern
}

// TestEngine_GracefulShutdown_HandlerDropsMessages verifies that handlers that
// drop messages (return nil) don't cause shutdown to hang.
func TestEngine_GracefulShutdown_HandlerDropsMessages(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 2 * time.Second,
	})

	var droppedCount atomic.Int32

	// Handler that drops every other message
	_ = engine.AddHandler("filter", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]FinalEvent, error) {
			if cmd.ID[0]%2 == 0 {
				droppedCount.Add(1)
				return nil, nil // Drop
			}
			return []FinalEvent{{ID: cmd.ID, Status: "kept"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send 10 messages (5 will be dropped)
	for i := 0; i < 10; i++ {
		data, _ := json.Marshal(Step1Command{ID: string(rune(i))})
		input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)
	}

	// Collect kept messages
	kept := 0
	timeout := time.After(2 * time.Second)
	for kept < 5 {
		select {
		case <-output:
			kept++
		case <-timeout:
			t.Fatalf("Timeout: expected 5 kept messages, got %d", kept)
		}
	}

	// Initiate shutdown
	close(input)
	cancel()

	select {
	case <-done:
		// Success - shutdown completed despite dropped messages
	case <-time.After(5 * time.Second):
		t.Fatal("Engine didn't shut down")
	}

	if droppedCount.Load() != 5 {
		t.Errorf("Expected 5 dropped messages, got %d", droppedCount.Load())
	}
}

// TestEngine_GracefulShutdown_HandlerMultipliesMessages verifies that multiplied
// messages (handler returns N > 1) don't cause shutdown to hang.
func TestEngine_GracefulShutdown_HandlerMultipliesMessages(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 2 * time.Second,
	})

	var outputCount atomic.Int32

	// Handler that produces 3 outputs per input
	_ = engine.AddHandler("fanout", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]FinalEvent, error) {
			return []FinalEvent{
				{ID: cmd.ID + "-a", Status: "copy-a"},
				{ID: cmd.ID + "-b", Status: "copy-b"},
				{ID: cmd.ID + "-c", Status: "copy-c"},
			}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send 5 messages (should produce 15 outputs)
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(Step1Command{ID: string(rune('A' + i))})
		input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)
	}

	// Collect all outputs
	timeout := time.After(2 * time.Second)
	for outputCount.Load() < 15 {
		select {
		case <-output:
			outputCount.Add(1)
		case <-timeout:
			t.Fatalf("Timeout: expected 15 outputs, got %d", outputCount.Load())
		}
	}

	// Initiate shutdown
	close(input)
	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Engine didn't shut down")
	}

	if outputCount.Load() != 15 {
		t.Errorf("Expected 15 outputs, got %d", outputCount.Load())
	}
}

// TestEngine_ShutdownTimeout_MarshalUnmarshalPipes verifies that marshal and
// unmarshal pipes respond to shutdown when context is cancelled, even if
// input channels are not closed. This tests that all components use consistent
// ShutdownTimeout behavior.
func TestEngine_ShutdownTimeout_MarshalUnmarshalPipes(t *testing.T) {
	shutdownTimeout := 100 * time.Millisecond

	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: shutdownTimeout,
	})

	// Simple pass-through handler
	_ = engine.AddHandler("passthrough", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]FinalEvent, error) {
			return []FinalEvent{{ID: cmd.ID, Status: "processed"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Raw input - we intentionally do NOT close this
	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)

	// Raw output - involves marshal pipe
	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send a message and receive it
	data, _ := json.Marshal(Step1Command{ID: "test-1"})
	input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)

	select {
	case <-output:
		// Got output, pipeline is working
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for first message")
	}

	// Cancel context WITHOUT closing input channel
	// This tests that unmarshal pipe responds to shutdown via context
	cancel()

	// Shutdown should complete within ShutdownTimeout + margin
	// With the fix, marshal/unmarshal see shutdown immediately and use their timeout
	select {
	case <-done:
		// Success - shutdown completed
	case <-time.After(shutdownTimeout + 500*time.Millisecond):
		t.Fatal("Shutdown took too long - marshal/unmarshal pipes may not be responding to context")
	}
}

// TestEngine_ShutdownTimeout_ImmediateShutdown verifies that ShutdownTimeout=0
// forces immediate shutdown of all components including marshal/unmarshal pipes.
func TestEngine_ShutdownTimeout_ImmediateShutdown(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 0, // Immediate shutdown
	})

	// Slow handler to ensure we have messages in flight
	_ = engine.AddHandler("slow", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]FinalEvent, error) {
			time.Sleep(50 * time.Millisecond)
			return []FinalEvent{{ID: cmd.ID, Status: "processed"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	input := make(chan *RawMessage, 100)
	_, _ = engine.AddRawInput("input", nil, input)
	_, _ = engine.AddRawOutput("output", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send many messages
	for i := 0; i < 50; i++ {
		data, _ := json.Marshal(Step1Command{ID: string(rune('A' + i))})
		input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)
	}

	// Give pipeline time to start processing
	time.Sleep(20 * time.Millisecond)

	// Cancel - with ShutdownTimeout=0, should be immediate
	cancel()

	// Should shutdown very quickly (no grace period)
	select {
	case <-done:
		// Success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Immediate shutdown took too long")
	}
}
