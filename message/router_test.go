package message_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
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
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
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
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
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
	pipe1 := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		out := message.Copy(msg, []byte("pipe1"))
		out.Attributes[message.AttrSubject] = "out1"
		return []*message.Message{out}, nil
	})

	pipe2 := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
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
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
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
	pipe := gopipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
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
	pipe := gopipe.NewTransformPipe(func(ctx context.Context, msg *message.Message) (*message.Message, error) {
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
