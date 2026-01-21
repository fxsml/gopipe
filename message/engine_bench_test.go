package message

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/channel"
)

// Benchmark types
type BenchCommand struct {
	ID int `json:"id"`
}

type BenchEvent struct {
	ID   int `json:"id"`
	Step int `json:"step"`
}

// Step-specific event types for MultiStep benchmark (each handler needs unique input type)
type BenchStep2Event struct {
	ID int `json:"id"`
}

type BenchStep3Event struct {
	ID int `json:"id"`
}

type BenchStep4Event struct {
	ID int `json:"id"`
}

type BenchFinalEvent struct {
	ID int `json:"id"`
}

type benchMatcher struct {
	pattern string
}

func (m *benchMatcher) Match(attrs Attributes) bool {
	t, _ := attrs["type"].(string)
	return t == m.pattern
}

// silentLogger suppresses all log output during benchmarks
type silentLogger struct{}

func (silentLogger) Debug(msg string, args ...any) {}
func (silentLogger) Info(msg string, args ...any)  {}
func (silentLogger) Warn(msg string, args ...any)  {}
func (silentLogger) Error(msg string, args ...any) {}

// BenchmarkEngine_Loopback_Throughput measures message throughput with a single loopback.
func BenchmarkEngine_Loopback_Throughput(b *testing.B) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
		BufferSize:      1000,
		Logger:          silentLogger{},
	})

	// Step 1: Command -> Event
	_ = engine.AddHandler("step1", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchCommand) ([]BenchEvent, error) {
			return []BenchEvent{{ID: cmd.ID, Step: 1}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	// Step 2: Event -> Final
	_ = engine.AddHandler("step2", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchEvent) ([]BenchFinalEvent, error) {
			return []BenchFinalEvent{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	// Loopback
	loopOut, _ := engine.AddLoopbackOutput("loop", &benchMatcher{pattern: "bench.event"})
	_, _ = engine.AddLoopbackInput("loop", nil, loopOut)

	input := make(chan *RawMessage, 1000)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &benchMatcher{pattern: "bench.final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Prepare messages
	msgs := make([]*RawMessage, b.N)
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(BenchCommand{ID: i})
		msgs[i] = NewRaw(data, Attributes{"type": "bench.command"}, nil)
	}

	b.ResetTimer()

	// Send all messages
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, msg := range msgs {
			input <- msg
		}
	}()

	// Receive all messages
	for i := 0; i < b.N; i++ {
		<-output
	}

	b.StopTimer()

	close(input)
	cancel()
	<-done
	wg.Wait()
}

// BenchmarkEngine_Loopback_MultiStep measures throughput with chained loopbacks.
// Flow: BenchCommand → step1 → BenchStep2Event → step2 → BenchStep3Event → step3 → BenchStep4Event → step4 → BenchFinalEvent
func BenchmarkEngine_Loopback_MultiStep(b *testing.B) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
		BufferSize:      1000,
		Logger:          silentLogger{},
	})

	// 4-step pipeline with unique event types per step
	_ = engine.AddHandler("step1", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchCommand) ([]BenchStep2Event, error) {
			return []BenchStep2Event{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	_ = engine.AddHandler("step2", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchStep2Event) ([]BenchStep3Event, error) {
			return []BenchStep3Event{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	_ = engine.AddHandler("step3", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchStep3Event) ([]BenchStep4Event, error) {
			return []BenchStep4Event{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	_ = engine.AddHandler("step4", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchStep4Event) ([]BenchFinalEvent, error) {
			return []BenchFinalEvent{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	// Create loopbacks for each step's output
	addBenchLoopback := func(name, pattern string) {
		out, _ := engine.AddLoopbackOutput(name, &benchMatcher{pattern: pattern})
		_, _ = engine.AddLoopbackInput(name, nil, out)
	}

	addBenchLoopback("loop2", "bench.step2.event")
	addBenchLoopback("loop3", "bench.step3.event")
	addBenchLoopback("loop4", "bench.step4.event")

	input := make(chan *RawMessage, 1000)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &benchMatcher{pattern: "bench.final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Prepare messages
	msgs := make([]*RawMessage, b.N)
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(BenchCommand{ID: i})
		msgs[i] = NewRaw(data, Attributes{"type": "bench.command"}, nil)
	}

	b.ResetTimer()

	// Send all messages
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, msg := range msgs {
			input <- msg
		}
	}()

	// Receive all messages
	for i := 0; i < b.N; i++ {
		<-output
	}

	b.StopTimer()

	close(input)
	cancel()
	<-done
	wg.Wait()
}

// BenchmarkEngine_Loopback_ShutdownLatency measures time from cancel to done with loopbacks.
func BenchmarkEngine_Loopback_ShutdownLatency(b *testing.B) {
	for i := 0; i < b.N; i++ {
		engine := NewEngine(EngineConfig{
			Marshaler:       NewJSONMarshaler(),
			ShutdownTimeout: 5 * time.Second,
			BufferSize:      100,
			Logger:          silentLogger{},
		})

		_ = engine.AddHandler("step1", nil, NewCommandHandler(
			func(ctx context.Context, cmd BenchCommand) ([]BenchEvent, error) {
				return []BenchEvent{{ID: cmd.ID, Step: 1}}, nil
			},
			CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
		))

		_ = engine.AddHandler("step2", nil, NewCommandHandler(
			func(ctx context.Context, cmd BenchEvent) ([]BenchFinalEvent, error) {
				return []BenchFinalEvent{{ID: cmd.ID}}, nil
			},
			CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
		))

		loopOut, _ := engine.AddLoopbackOutput("loop", &benchMatcher{pattern: "bench.event"})
		_, _ = engine.AddLoopbackInput("loop", nil, loopOut)

		input := make(chan *RawMessage, 100)
		_, _ = engine.AddRawInput("input", nil, input)

		output, _ := engine.AddRawOutput("output", &benchMatcher{pattern: "bench.final.event"})

		ctx, cancel := context.WithCancel(context.Background())
		done, _ := engine.Start(ctx)

		// Send some messages
		for j := 0; j < 100; j++ {
			data, _ := json.Marshal(BenchCommand{ID: j})
			input <- NewRaw(data, Attributes{"type": "bench.command"}, nil)
		}

		// Consume outputs
		for j := 0; j < 100; j++ {
			<-output
		}

		// Measure shutdown
		close(input)
		b.StartTimer()
		cancel()
		<-done
		b.StopTimer()
	}
}

// BenchmarkEngine_Loopback_ProcessLoopback measures throughput with transformation.
func BenchmarkEngine_Loopback_ProcessLoopback(b *testing.B) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
		BufferSize:      1000,
		Logger:          silentLogger{},
	})

	_ = engine.AddHandler("step1", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchCommand) ([]BenchEvent, error) {
			return []BenchEvent{{ID: cmd.ID, Step: 1}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	_ = engine.AddHandler("step2", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchFinalEvent) ([]BenchFinalEvent, error) {
			return []BenchFinalEvent{cmd}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	// ProcessLoopback with transformation
	loopOut, _ := engine.AddLoopbackOutput("loop", &benchMatcher{pattern: "bench.event"})
	processed := channel.Process(loopOut, func(msg *Message) []*Message {
		event := msg.Data.(BenchEvent)
		return []*Message{{
			Data:       BenchFinalEvent{ID: event.ID},
			Attributes: Attributes{"type": "bench.final.event"},
		}}
	})
	_, _ = engine.AddLoopbackInput("loop", nil, processed)

	input := make(chan *RawMessage, 1000)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &benchMatcher{pattern: "bench.final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Prepare messages
	msgs := make([]*RawMessage, b.N)
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(BenchCommand{ID: i})
		msgs[i] = NewRaw(data, Attributes{"type": "bench.command"}, nil)
	}

	b.ResetTimer()

	// Send all messages
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, msg := range msgs {
			input <- msg
		}
	}()

	// Receive all messages
	for i := 0; i < b.N; i++ {
		<-output
	}

	b.StopTimer()

	close(input)
	cancel()
	<-done
	wg.Wait()
}

// BenchmarkEngine_NoLoopback_Baseline measures throughput without loopback for comparison.
func BenchmarkEngine_NoLoopback_Baseline(b *testing.B) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
		BufferSize:      1000,
		Logger:          silentLogger{},
	})

	// Single handler, no loopback
	_ = engine.AddHandler("handler", nil, NewCommandHandler(
		func(ctx context.Context, cmd BenchCommand) ([]BenchFinalEvent, error) {
			return []BenchFinalEvent{{ID: cmd.ID}}, nil
		},
		CommandHandlerConfig{Source: "/bench", Naming: KebabNaming},
	))

	input := make(chan *RawMessage, 1000)
	_, _ = engine.AddRawInput("input", nil, input)

	output, _ := engine.AddRawOutput("output", &benchMatcher{pattern: "bench.final.event"})

	ctx, cancel := context.WithCancel(context.Background())
	done, _ := engine.Start(ctx)

	// Prepare messages
	msgs := make([]*RawMessage, b.N)
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(BenchCommand{ID: i})
		msgs[i] = NewRaw(data, Attributes{"type": "bench.command"}, nil)
	}

	b.ResetTimer()

	// Send all messages
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, msg := range msgs {
			input <- msg
		}
	}()

	// Receive all messages
	for i := 0; i < b.N; i++ {
		<-output
	}

	b.StopTimer()

	close(input)
	cancel()
	<-done
	wg.Wait()
}
