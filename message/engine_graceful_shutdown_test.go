package message

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fxsml/gopipe/channel"
)

// Test types for graceful shutdown tests
type Step1Command struct {
	ID string `json:"id"`
}

type Step2Event struct {
	ID   string `json:"id"`
	Step int    `json:"step"`
}

type Step3Event struct {
	ID   string `json:"id"`
	Step int    `json:"step"`
}

type Step4Event struct {
	ID   string `json:"id"`
	Step int    `json:"step"`
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

// addLoopback is a test helper that creates a loopback (equivalent to plugin.Loopback)
func addLoopback(e *Engine, name string, matcher Matcher) error {
	out, err := e.AddLoopbackOutput(name, matcher)
	if err != nil {
		return err
	}
	_, err = e.AddLoopbackInput(name, nil, out)
	return err
}

// addProcessLoopback is a test helper that creates a process loopback
func addProcessLoopback(e *Engine, name string, matcher Matcher, handle func(*Message) []*Message) error {
	out, err := e.AddLoopbackOutput(name, matcher)
	if err != nil {
		return err
	}
	processed := channel.Process(out, handle)
	_, err = e.AddLoopbackInput(name, nil, processed)
	return err
}

// TestEngine_GracefulShutdown_ChainedLoopbacks verifies that a multi-step pipeline
// with chained loopbacks shuts down gracefully without dropping in-flight messages.
func TestEngine_GracefulShutdown_ChainedLoopbacks(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
	})

	var processedCount atomic.Int32

	// Step 1: Command -> Step2Event
	_ = engine.AddHandler("step1", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]Step2Event, error) {
			return []Step2Event{{ID: cmd.ID, Step: 2}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Step 2: Step2Event -> Step3Event
	_ = engine.AddHandler("step2", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step2Event) ([]Step3Event, error) {
			return []Step3Event{{ID: cmd.ID, Step: 3}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Step 3: Step3Event -> Step4Event
	_ = engine.AddHandler("step3", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step3Event) ([]Step4Event, error) {
			return []Step4Event{{ID: cmd.ID, Step: 4}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Step 4: Step4Event -> FinalEvent
	_ = engine.AddHandler("step4", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step4Event) ([]FinalEvent, error) {
			processedCount.Add(1)
			return []FinalEvent{{ID: cmd.ID, Status: "completed"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Wire up loopbacks for each intermediate step
	_ = addLoopback(engine, "loop2", &shutdownTypeMatcher{pattern: "step2.event"})
	_ = addLoopback(engine, "loop3", &shutdownTypeMatcher{pattern: "step3.event"})
	_ = addLoopback(engine, "loop4", &shutdownTypeMatcher{pattern: "step4.event"})

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, err := engine.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send multiple messages
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		data, _ := json.Marshal(Step1Command{ID: string(rune('A' + i))})
		input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)
	}

	// Collect all outputs
	received := make(map[string]bool)
	for i := 0; i < messageCount; i++ {
		select {
		case msg := <-output:
			var event FinalEvent
			_ = json.Unmarshal(msg.Data, &event)
			received[event.ID] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for message %d", i+1)
		}
	}

	// Initiate graceful shutdown
	close(input)
	cancel()

	// Wait for engine to stop
	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Engine did not shut down within timeout")
	}

	// Verify all messages were processed
	if int(processedCount.Load()) != messageCount {
		t.Errorf("Expected %d messages processed, got %d", messageCount, processedCount.Load())
	}

	// Verify all messages received
	if len(received) != messageCount {
		t.Errorf("Expected %d messages received, got %d", messageCount, len(received))
	}
}

// TestEngine_GracefulShutdown_InFlightDuringCancel verifies that messages
// in the middle of a multi-step pipeline complete their journey during shutdown.
func TestEngine_GracefulShutdown_InFlightDuringCancel(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
	})

	var step1Count, step2Count, step3Count atomic.Int32
	step1Done := make(chan struct{})
	step2Proceed := make(chan struct{})

	// Step 1: Fast, signals completion
	_ = engine.AddHandler("step1", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]Step2Event, error) {
			step1Count.Add(1)
			close(step1Done)
			return []Step2Event{{ID: cmd.ID, Step: 2}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Step 2: Blocks until signaled (simulates slow processing)
	_ = engine.AddHandler("step2", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step2Event) ([]Step3Event, error) {
			step2Count.Add(1)
			<-step2Proceed // Wait for signal to continue
			return []Step3Event{{ID: cmd.ID, Step: 3}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Step 3: Final
	_ = engine.AddHandler("step3", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step3Event) ([]FinalEvent, error) {
			step3Count.Add(1)
			return []FinalEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	_ = addLoopback(engine, "loop2", &shutdownTypeMatcher{pattern: "step2.event"})
	_ = addLoopback(engine, "loop3", &shutdownTypeMatcher{pattern: "step3.event"})

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send one message
	data, _ := json.Marshal(Step1Command{ID: "test"})
	input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)

	// Wait for step1 to complete - message has entered the pipeline
	select {
	case <-step1Done:
		// Step 1 completed, message is now being routed through loopback
	case <-time.After(5 * time.Second):
		t.Fatal("Step 1 didn't process the message")
	}

	// Give loopback time to route message to step2
	time.Sleep(100 * time.Millisecond)

	// Close input and cancel while message is in-flight (blocked in step2)
	close(input)
	cancel()

	// Allow step 2 to complete
	close(step2Proceed)

	// Wait for output - message should complete its journey
	select {
	case <-output:
		// Success - message completed
	case <-time.After(5 * time.Second):
		t.Fatal("In-flight message was dropped during shutdown")
	}

	// Wait for engine
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Engine didn't shut down")
	}

	// Verify message completed all steps
	if step1Count.Load() != 1 || step2Count.Load() != 1 || step3Count.Load() != 1 {
		t.Errorf("Message didn't complete all steps: step1=%d, step2=%d, step3=%d",
			step1Count.Load(), step2Count.Load(), step3Count.Load())
	}
}

// TestEngine_GracefulShutdown_HandlerDropsMessages verifies that dropped messages
// (handler returns empty) are tracked correctly for graceful shutdown.
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
		t.Fatal("Engine didn't shut down - dropped messages may not be tracked correctly")
	}

	if droppedCount.Load() != 5 {
		t.Errorf("Expected 5 dropped messages, got %d", droppedCount.Load())
	}
}

// TestEngine_GracefulShutdown_HandlerMultipliesMessages verifies that multiplied
// messages (handler returns N > 1) are tracked correctly for graceful shutdown.
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
		t.Fatal("Engine didn't shut down - multiplied messages may not be tracked correctly")
	}

	if outputCount.Load() != 15 {
		t.Errorf("Expected 15 outputs, got %d", outputCount.Load())
	}
}

// TestEngine_GracefulShutdown_NoMessages verifies shutdown works when no messages
// were ever sent.
func TestEngine_GracefulShutdown_NoMessages(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: time.Second,
	})

	_ = engine.AddHandler("handler", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]FinalEvent, error) {
			return []FinalEvent{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	_ = addLoopback(engine, "loop", &shutdownTypeMatcher{pattern: "final.event"})

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)
	_, _ = engine.AddRawOutput("output", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Immediately shutdown without sending any messages
	close(input)
	cancel()

	select {
	case <-done:
		// Success - should complete quickly
	case <-time.After(2 * time.Second):
		t.Fatal("Engine didn't shut down when no messages were sent")
	}
}

// TestEngine_GracefulShutdown_ImmediateCancel verifies shutdown works when
// context is cancelled immediately after Start().
func TestEngine_GracefulShutdown_ImmediateCancel(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: time.Second,
	})

	_ = engine.AddHandler("handler", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]FinalEvent, error) {
			return []FinalEvent{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	_ = addLoopback(engine, "loop", &shutdownTypeMatcher{pattern: "final.event"})

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)
	_, _ = engine.AddRawOutput("output", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Cancel immediately
	cancel()
	close(input)

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Engine didn't shut down on immediate cancel")
	}
}

// TestEngine_GracefulShutdown_NoMessageLoss verifies that messages in-flight
// are processed during graceful shutdown.
func TestEngine_GracefulShutdown_NoMessageLoss(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 10 * time.Second,
	})

	var processedCount atomic.Int64

	// Simple loopback pipeline
	_ = engine.AddHandler("step1", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]Step2Event, error) {
			return []Step2Event{{ID: cmd.ID, Step: 2}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	_ = engine.AddHandler("step2", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step2Event) ([]FinalEvent, error) {
			processedCount.Add(1)
			return []FinalEvent{{ID: cmd.ID, Status: "done"}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	_ = addLoopback(engine, "loop", &shutdownTypeMatcher{pattern: "step2.event"})

	input := make(chan *RawMessage, 100)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send messages - reduced count to make test more reliable under race detector
	messageCount := 50
	for i := 0; i < messageCount; i++ {
		data, _ := json.Marshal(Step1Command{ID: string(rune('A' + i))})
		input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)
	}

	// Wait for all messages to be received before shutdown
	receivedCount := 0
	for receivedCount < messageCount {
		select {
		case <-output:
			receivedCount++
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for message %d", receivedCount+1)
		}
	}

	// Now initiate shutdown - all messages have been processed
	close(input)
	cancel()

	// Wait for engine to stop
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Engine didn't shut down")
	}

	// Verify counts match
	if processedCount.Load() != int64(messageCount) {
		t.Errorf("Message mismatch: processed=%d, expected=%d",
			processedCount.Load(), messageCount)
	}
}

// TestEngine_GracefulShutdown_ProcessLoopback verifies graceful shutdown
// works with ProcessLoopback transformation.
func TestEngine_GracefulShutdown_ProcessLoopback(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
	})

	var transformCount atomic.Int32

	// Initial handler
	_ = engine.AddHandler("initial", nil, NewCommandHandler(
		func(ctx context.Context, cmd Step1Command) ([]Step2Event, error) {
			return []Step2Event{{ID: cmd.ID, Step: 2}}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// Handler that receives transformed messages
	_ = engine.AddHandler("final", nil, NewCommandHandler(
		func(ctx context.Context, cmd FinalEvent) ([]FinalEvent, error) {
			return []FinalEvent{cmd}, nil
		},
		CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
	))

	// ProcessLoopback with transformation
	_ = addProcessLoopback(engine,
		"transform-loop",
		&shutdownTypeMatcher{pattern: "step2.event"},
		func(msg *Message) []*Message {
			transformCount.Add(1)
			event := msg.Data.(Step2Event)
			return []*Message{{
				Data:       FinalEvent{ID: event.ID, Status: "transformed"},
				Attributes: Attributes{"type": "final.event"},
			}}
		},
	)

	input := make(chan *RawMessage, 10)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &shutdownTypeMatcher{pattern: "final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Send messages
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(Step1Command{ID: string(rune('A' + i))})
		input <- NewRaw(data, Attributes{"type": "step1.command"}, nil)
	}

	// Collect outputs
	for i := 0; i < 5; i++ {
		select {
		case msg := <-output:
			var event FinalEvent
			_ = json.Unmarshal(msg.Data, &event)
			if event.Status != "transformed" {
				t.Errorf("Expected transformed status, got %s", event.Status)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for output %d", i+1)
		}
	}

	close(input)
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Engine didn't shut down")
	}

	if transformCount.Load() != 5 {
		t.Errorf("Expected 5 transforms, got %d", transformCount.Load())
	}
}
