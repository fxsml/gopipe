package message

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// Benchmark types
type BenchCommand struct {
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

// BenchmarkEngine_Throughput measures message throughput.
func BenchmarkEngine_Throughput(b *testing.B) {
	engine := NewEngine(EngineConfig{
		Marshaler:       NewJSONMarshaler(),
		ShutdownTimeout: 5 * time.Second,
		BufferSize:      1000,
		Logger:          silentLogger{},
	})

	// Single handler
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
