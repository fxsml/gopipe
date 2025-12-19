package message_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

func TestRouter_BasicRouting(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, append(msg.Data, []byte("-processed")...))
			out.Attributes[message.AttrSubject] = "processed"
			return []*message.Message{out}, nil
		},
		message.MatchSubject("input"),
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	in := channel.FromValues(
		message.New([]byte("test"), message.Attributes{message.AttrSubject: "input"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if string(results[0].Data) != "test-processed" {
		t.Errorf("Expected 'test-processed', got %s", string(results[0].Data))
	}

	subject, _ := results[0].Attributes.Subject()
	if subject != "processed" {
		t.Errorf("Expected subject 'processed', got %s", subject)
	}
}

func TestRouter_MultipleHandlers(t *testing.T) {
	handler1 := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, []byte("handler1"))
			out.Attributes[message.AttrSubject] = "out1"
			return []*message.Message{out}, nil
		},
		message.MatchSubject("type1"),
	)

	handler2 := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, []byte("handler2"))
			out.Attributes[message.AttrSubject] = "out2"
			return []*message.Message{out}, nil
		},
		message.MatchSubject("type2"),
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler1)
	router.AddHandler(handler2)

	in := channel.FromValues(
		message.New([]byte("a"), message.Attributes{message.AttrSubject: "type1"}),
		message.New([]byte("b"), message.Attributes{message.AttrSubject: "type2"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]int)
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		results[subject]++
	}

	if results["out1"] != 1 {
		t.Errorf("Expected 1 out1, got %d", results["out1"])
	}
	if results["out2"] != 1 {
		t.Errorf("Expected 1 out2, got %d", results["out2"])
	}
}

func TestRouter_NoMatchingHandler(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		message.MatchSubject("expected"),
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	var nackCalled bool
	var nackErr error

	in := channel.FromValues(
		message.NewWithAcking([]byte("test"), message.Attributes{
			message.AttrSubject: "unknown",
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
	if nackErr == nil {
		t.Error("Expected nack error to be set")
	}
}

func TestRouter_HandlerError(t *testing.T) {
	testErr := errors.New("handler error")
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			msg.Nack(testErr)
			return nil, testErr
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	var nackCalled bool

	in := channel.FromValues(
		message.NewWithAcking([]byte("test"), nil,
			func() {},
			func(err error) {
				nackCalled = true
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
}

func TestRouter_AckOnSuccess(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			msg.Ack()
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	var ackCalled bool

	in := channel.FromValues(
		message.NewWithAcking([]byte("test"), nil,
			func() { ackCalled = true },
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
	var processedCount int

	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			processedCount++
			mu.Unlock()
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{
		Concurrency: 5,
	})
	router.AddHandler(handler)

	in := make(chan *message.Message, 10)
	for i := 0; i < 10; i++ {
		in <- message.New([]byte("test"), nil)
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
	if processedCount != 10 {
		t.Errorf("Expected 10 processed, got %d", processedCount)
	}
	mu.Unlock()
}

func TestRouter_WithRecover(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			panic("handler panic")
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{
		Recover: true,
	})
	router.AddHandler(handler)

	in := channel.FromValues(message.New([]byte("test"), nil))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	// Should not panic due to Recover option
	for range out {
	}
}

func TestRouter_MultipleOutputMessages(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{
				message.Copy(msg, []byte("out1")),
				message.Copy(msg, []byte("out2")),
			}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	in := channel.FromValues(message.New([]byte("input"), nil))

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

func TestRouter_AddPipe_Basic(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, append(msg.Data, []byte("-piped")...))
		out.Attributes[message.AttrSubject] = "piped"
		return []*message.Message{out}, nil
	})

	router := message.NewRouter(message.RouterConfig{})
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("pipe-input")))

	in := channel.FromValues(
		message.New([]byte("test"), message.Attributes{message.AttrSubject: "pipe-input"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if string(results[0].Data) != "test-piped" {
		t.Errorf("Expected 'test-piped', got %s", string(results[0].Data))
	}

	subject, _ := results[0].Attributes.Subject()
	if subject != "piped" {
		t.Errorf("Expected subject 'piped', got %s", subject)
	}
}

func TestRouter_AddPipe_WithHandlers(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, []byte("from-pipe"))
		out.Attributes[message.AttrSubject] = "pipe-out"
		return []*message.Message{out}, nil
	})

	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, []byte("from-handler"))
			out.Attributes[message.AttrSubject] = "handler-out"
			return []*message.Message{out}, nil
		},
		message.MatchSubject("handler-input"),
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("pipe-input")))

	in := channel.FromValues(
		message.New([]byte("a"), message.Attributes{message.AttrSubject: "pipe-input"}),
		message.New([]byte("b"), message.Attributes{message.AttrSubject: "handler-input"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]int)
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		results[subject]++
	}

	if results["pipe-out"] != 1 {
		t.Errorf("Expected 1 pipe-out, got %d", results["pipe-out"])
	}
	if results["handler-out"] != 1 {
		t.Errorf("Expected 1 handler-out, got %d", results["handler-out"])
	}
}

func TestRouter_AddPipe_MultiplePipes(t *testing.T) {
	pipe1 := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, []byte("pipe1"))
		out.Attributes[message.AttrSubject] = "out1"
		return []*message.Message{out}, nil
	})

	pipe2 := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, []byte("pipe2"))
		out.Attributes[message.AttrSubject] = "out2"
		return []*message.Message{out}, nil
	})

	router := message.NewRouter(message.RouterConfig{})
	router.AddPipe(message.NewPipe(pipe1, message.MatchSubject("in1")))
	router.AddPipe(message.NewPipe(pipe2, message.MatchSubject("in2")))

	in := channel.FromValues(
		message.New([]byte("a"), message.Attributes{message.AttrSubject: "in1"}),
		message.New([]byte("b"), message.Attributes{message.AttrSubject: "in2"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	results := make(map[string]string)
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		results[subject] = string(msg.Data)
	}

	if results["out1"] != "pipe1" {
		t.Errorf("Expected 'pipe1' for out1, got %s", results["out1"])
	}
	if results["out2"] != "pipe2" {
		t.Errorf("Expected 'pipe2' for out2, got %s", results["out2"])
	}
}

func TestRouter_AddPipe_NoMatch(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		t.Error("Pipe should not be called for non-matching message")
		return []*message.Message{msg}, nil
	})

	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, []byte("handled"))
			out.Attributes[message.AttrSubject] = "handled"
			return []*message.Message{out}, nil
		},
		message.MatchSubject("handler-input"),
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("pipe-input")))

	in := channel.FromValues(
		message.New([]byte("test"), message.Attributes{message.AttrSubject: "handler-input"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for msg := range out {
		subject, _ := msg.Attributes.Subject()
		if subject != "handled" {
			t.Errorf("Expected subject 'handled', got %s", subject)
		}
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 result, got %d", count)
	}
}

func TestRouter_AddPipe_MultipleOutputs(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out1 := message.Copy(msg, []byte("out1"))
		out2 := message.Copy(msg, []byte("out2"))
		return []*message.Message{out1, out2}, nil
	})

	router := message.NewRouter(message.RouterConfig{})
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("input")))

	in := channel.FromValues(
		message.New([]byte("test"), message.Attributes{message.AttrSubject: "input"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
}

func TestRouter_AddHandlerAfterStart(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})

	// AddHandler before Start should succeed
	ok := router.AddHandler(handler)
	if !ok {
		t.Error("AddHandler before Start should return true")
	}

	// Start the router
	in := make(chan *message.Message)
	close(in)
	router.Start(context.Background(), in)

	// AddHandler after Start should fail
	ok = router.AddHandler(handler)
	if ok {
		t.Error("AddHandler after Start should return false")
	}
}

func TestRouter_AddPipeAfterStart(t *testing.T) {
	pipe := pipe.NewTransformPipe(func(ctx context.Context, msg *message.Message) (*message.Message, error) {
		return msg, nil
	})

	router := message.NewRouter(message.RouterConfig{})

	// AddPipe before Start should succeed
	ok := router.AddPipe(message.NewPipe(pipe, func(attrs message.Attributes) bool { return false }))
	if !ok {
		t.Error("AddPipe before Start should return true")
	}

	// Start the router
	in := make(chan *message.Message)
	close(in)
	router.Start(context.Background(), in)

	// AddPipe after Start should fail
	ok = router.AddPipe(message.NewPipe(pipe, func(attrs message.Attributes) bool { return false }))
	if ok {
		t.Error("AddPipe after Start should return false")
	}
}

func TestRouter_StartTwice(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	// First Start should succeed
	in1 := make(chan *message.Message)
	close(in1)
	out1 := router.Start(context.Background(), in1)
	if out1 == nil {
		t.Error("first Start should return non-nil channel")
	}

	// Second Start should return nil
	in2 := make(chan *message.Message)
	close(in2)
	out2 := router.Start(context.Background(), in2)
	if out2 != nil {
		t.Error("second Start should return nil")
	}
}

func TestRouter_MatchCombination(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		message.Match(
			message.MatchSubject("orders"),
			message.MatchType("command"),
		),
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	var ackCount, nackCount int
	var mu sync.Mutex

	msgs := []*message.Message{
		// Matches both
		message.NewWithAcking([]byte("1"), message.Attributes{
			message.AttrSubject: "orders",
			message.AttrType:    "command",
		}, func() {
			mu.Lock()
			ackCount++
			mu.Unlock()
		}, func(err error) {
			mu.Lock()
			nackCount++
			mu.Unlock()
		}),
		// Only matches subject
		message.NewWithAcking([]byte("2"), message.Attributes{
			message.AttrSubject: "orders",
			message.AttrType:    "event",
		}, func() {
			mu.Lock()
			ackCount++
			mu.Unlock()
		}, func(err error) {
			mu.Lock()
			nackCount++
			mu.Unlock()
		}),
		// Only matches type
		message.NewWithAcking([]byte("3"), message.Attributes{
			message.AttrSubject: "users",
			message.AttrType:    "command",
		}, func() {
			mu.Lock()
			ackCount++
			mu.Unlock()
		}, func(err error) {
			mu.Lock()
			nackCount++
			mu.Unlock()
		}),
	}

	in := channel.FromValues(msgs...)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 result (only full match), got %d", count)
	}

	mu.Lock()
	defer mu.Unlock()
	// Handler calls Ack on success for one message
	// Note: The handler in this test doesn't call Ack explicitly, so we check nacks
	if nackCount != 2 {
		t.Errorf("Expected 2 nacks (partial matches), got %d", nackCount)
	}
}

func TestRouter_NoHandlers(t *testing.T) {
	router := message.NewRouter(message.RouterConfig{})
	// No handlers added

	var nackCalled bool
	in := channel.FromValues(
		message.NewWithAcking([]byte("test"), message.Attributes{message.AttrSubject: "test"},
			func() {},
			func(err error) { nackCalled = true }),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 results with no handlers, got %d", count)
	}
	if !nackCalled {
		t.Error("Expected nack to be called when no handler matches")
	}
}

func TestRouter_EmptyInput(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	// Empty input channel
	in := make(chan *message.Message)
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 results with empty input, got %d", count)
	}
}

func TestRouter_OnlyPipes_NoHandlers(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, []byte("piped"))
		return []*message.Message{out}, nil
	})

	router := message.NewRouter(message.RouterConfig{})
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("pipe-input")))
	// No handlers added

	in := channel.FromValues(
		message.New([]byte("test"), message.Attributes{message.AttrSubject: "pipe-input"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result from pipe, got %d", len(results))
	}

	if string(results[0].Data) != "piped" {
		t.Errorf("Expected 'piped', got %s", string(results[0].Data))
	}
}

func TestRouter_OnlyPipes_MessageNotMatchingPipe(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, []byte("piped"))
		return []*message.Message{out}, nil
	})

	router := message.NewRouter(message.RouterConfig{})
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("pipe-input")))
	// No handlers added

	// Message doesn't match pipe - should fall through to handler (which doesn't exist)
	var nackCalled bool
	in := channel.FromValues(
		message.NewWithAcking([]byte("test"), message.Attributes{message.AttrSubject: "other-input"},
			func() {},
			func(err error) { nackCalled = true }),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 results when message doesn't match pipe, got %d", count)
	}
	if !nackCalled {
		t.Error("Expected nack when message doesn't match any pipe and no handlers exist")
	}
}

func TestRouter_NilInput(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Nil input channel
	out := router.Start(ctx, nil)

	// Should return nil with no generators
	if out != nil {
		t.Error("Expected nil output when input is nil and no generators")
	}
}

// simpleGenerator implements message.Generator for testing
type simpleGenerator struct {
	msgs []*message.Message
}

func (g *simpleGenerator) Generate(ctx context.Context) <-chan *message.Message {
	out := make(chan *message.Message)
	go func() {
		defer close(out)
		for _, msg := range g.msgs {
			select {
			case <-ctx.Done():
				return
			case out <- msg:
			}
		}
	}()
	return out
}

func TestRouter_AddGenerator_Basic(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, append(msg.Data, []byte("-processed")...))
			return []*message.Message{out}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	gen := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("gen1"), message.Attributes{message.AttrSubject: "test"}),
			message.New([]byte("gen2"), message.Attributes{message.AttrSubject: "test"}),
		},
	}

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddGenerator(gen)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start with nil input - generator should still work
	out := router.Start(ctx, nil)

	var results []*message.Message
	for msg := range out {
		results = append(results, msg)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results from generator, got %d", len(results))
	}

	if string(results[0].Data) != "gen1-processed" && string(results[1].Data) != "gen1-processed" {
		t.Error("Expected 'gen1-processed' in results")
	}
	if string(results[0].Data) != "gen2-processed" && string(results[1].Data) != "gen2-processed" {
		t.Error("Expected 'gen2-processed' in results")
	}
}

func TestRouter_AddGenerator_WithInput(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, append(msg.Data, []byte("-processed")...))
			return []*message.Message{out}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	gen := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("gen"), message.Attributes{message.AttrSubject: "test"}),
		},
	}

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddGenerator(gen)

	// Input channel also provides messages
	in := channel.FromValues(
		message.New([]byte("input"), message.Attributes{message.AttrSubject: "test"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, in)

	var results []string
	for msg := range out {
		results = append(results, string(msg.Data))
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results (1 from input, 1 from generator), got %d", len(results))
	}

	hasInput := false
	hasGen := false
	for _, r := range results {
		if r == "input-processed" {
			hasInput = true
		}
		if r == "gen-processed" {
			hasGen = true
		}
	}

	if !hasInput {
		t.Error("Expected 'input-processed' in results")
	}
	if !hasGen {
		t.Error("Expected 'gen-processed' in results")
	}
}

func TestRouter_AddGenerator_Multiple(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	gen1 := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("gen1"), message.Attributes{message.AttrSubject: "test"}),
		},
	}
	gen2 := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("gen2"), message.Attributes{message.AttrSubject: "test"}),
		},
	}

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddGenerator(gen1)
	router.AddGenerator(gen2)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, nil)

	var results []string
	for msg := range out {
		results = append(results, string(msg.Data))
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results from generators, got %d", len(results))
	}

	hasGen1 := false
	hasGen2 := false
	for _, r := range results {
		if r == "gen1" {
			hasGen1 = true
		}
		if r == "gen2" {
			hasGen2 = true
		}
	}

	if !hasGen1 || !hasGen2 {
		t.Errorf("Expected both 'gen1' and 'gen2' in results, got %v", results)
	}
}

func TestRouter_AddGenerator_WithPipe(t *testing.T) {
	pipe := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, append(msg.Data, []byte("-piped")...))
		return []*message.Message{out}, nil
	})

	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			out := message.Copy(msg, append(msg.Data, []byte("-handled")...))
			return []*message.Message{out}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	gen := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("gen-pipe"), message.Attributes{message.AttrSubject: "pipe-input"}),
			message.New([]byte("gen-handler"), message.Attributes{message.AttrSubject: "other"}),
		},
	}

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddPipe(message.NewPipe(pipe, message.MatchSubject("pipe-input")))
	router.AddGenerator(gen)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := router.Start(ctx, nil)

	results := make(map[string]bool)
	for msg := range out {
		results[string(msg.Data)] = true
	}

	if !results["gen-pipe-piped"] {
		t.Error("Expected 'gen-pipe-piped' in results")
	}
	if !results["gen-handler-handled"] {
		t.Error("Expected 'gen-handler-handled' in results")
	}
}

func TestRouter_AddGeneratorAfterStart(t *testing.T) {
	gen := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("test"), nil),
		},
	}

	router := message.NewRouter(message.RouterConfig{})

	// AddGenerator before Start should succeed
	ok := router.AddGenerator(gen)
	if !ok {
		t.Error("AddGenerator before Start should return true")
	}

	// Start the router
	in := make(chan *message.Message)
	close(in)
	router.Start(context.Background(), in)

	// AddGenerator after Start should fail
	ok = router.AddGenerator(gen)
	if ok {
		t.Error("AddGenerator after Start should return false")
	}
}

func TestRouter_NilInputWithGenerator(t *testing.T) {
	handler := message.NewHandler(
		func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			return []*message.Message{msg}, nil
		},
		func(attrs message.Attributes) bool { return true },
	)

	gen := &simpleGenerator{
		msgs: []*message.Message{
			message.New([]byte("from-generator"), nil),
		},
	}

	router := message.NewRouter(message.RouterConfig{})
	router.AddHandler(handler)
	router.AddGenerator(gen)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// With a generator, nil input should still work
	out := router.Start(ctx, nil)

	if out == nil {
		t.Fatal("Expected non-nil output when generator is present")
	}

	var count int
	for range out {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 result from generator, got %d", count)
	}
}
