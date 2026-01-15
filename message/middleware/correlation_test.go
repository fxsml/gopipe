package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/middleware"
)

func TestMiddlewareName_CorrelationID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	engine := message.NewEngine(message.EngineConfig{
		Logger: logger,
	})
	_ = engine.Use(middleware.CorrelationID())

	if !strings.Contains(buf.String(), "middleware=CorrelationID") {
		t.Errorf("expected log to contain 'middleware=CorrelationID', got: %s", buf.String())
	}
}

func TestCorrelationID(t *testing.T) {
	t.Run("propagates correlationid to outputs", func(t *testing.T) {
		engine := message.NewEngine(message.EngineConfig{
			BufferSize: 10,
		})

		// Register middleware
		if err := engine.Use(middleware.CorrelationID()); err != nil {
			t.Fatalf("Use() failed: %v", err)
		}

		// Handler that produces output messages
		handler := message.NewHandler[TestCommand](func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			cmd := msg.Data.(*TestCommand)
			return []*message.Message{
				{Data: &TestEvent{Value: cmd.Input * 2}, Attributes: message.Attributes{"type": "test.event"}},
				{Data: &TestEvent{Value: cmd.Input * 3}, Attributes: message.Attributes{"type": "test.event"}},
			}, nil
		}, message.KebabNaming)

		if err := engine.AddHandler("test", nil, handler); err != nil {
			t.Fatalf("AddHandler failed: %v", err)
		}

		input := make(chan *message.Message, 1)
		if _, err := engine.AddInput("input", nil, input); err != nil {
			t.Fatalf("AddInput failed: %v", err)
		}

		output, err := engine.AddOutput("output", nil)
		if err != nil {
			t.Fatalf("AddOutput failed: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, err := engine.Start(ctx)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Send message with correlationid
		input <- &message.Message{
			Data: &TestCommand{Input: 5},
			Attributes: message.Attributes{
				"type":                   "test.command",
				message.AttrCorrelationID: "abc-123",
			},
		}
		close(input)

		// Collect 2 outputs
		var outputs []*message.Message
		timeout := time.After(time.Second)
		for i := 0; i < 2; i++ {
			select {
			case msg := <-output:
				outputs = append(outputs, msg)
			case <-timeout:
				t.Fatalf("timeout waiting for output %d", i+1)
			}
		}

		cancel()
		<-done

		for i, out := range outputs {
			cid, ok := out.Attributes[message.AttrCorrelationID].(string)
			if !ok || cid != "abc-123" {
				t.Errorf("output[%d]: expected correlationid 'abc-123', got %v", i, out.Attributes[message.AttrCorrelationID])
			}
		}
	})

	t.Run("no correlationid does not add attribute", func(t *testing.T) {
		engine := message.NewEngine(message.EngineConfig{
			BufferSize: 10,
		})

		if err := engine.Use(middleware.CorrelationID()); err != nil {
			t.Fatalf("Use() failed: %v", err)
		}

		handler := message.NewHandler[TestCommand](func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			cmd := msg.Data.(*TestCommand)
			return []*message.Message{
				{Data: &TestEvent{Value: cmd.Input}, Attributes: message.Attributes{"type": "test.event"}},
			}, nil
		}, message.KebabNaming)

		if err := engine.AddHandler("test", nil, handler); err != nil {
			t.Fatalf("AddHandler failed: %v", err)
		}

		input := make(chan *message.Message, 1)
		if _, err := engine.AddInput("input", nil, input); err != nil {
			t.Fatalf("AddInput failed: %v", err)
		}

		output, err := engine.AddOutput("output", nil)
		if err != nil {
			t.Fatalf("AddOutput failed: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, err := engine.Start(ctx)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Send message without correlationid
		input <- &message.Message{
			Data: &TestCommand{Input: 5},
			Attributes: message.Attributes{
				"type": "test.command",
			},
		}
		close(input)

		select {
		case msg := <-output:
			if _, ok := msg.Attributes[message.AttrCorrelationID]; ok {
				t.Error("expected no correlationid attribute")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for output")
		}

		cancel()
		<-done
	})
}

func TestUse_AfterStart(t *testing.T) {
	engine := message.NewEngine(message.EngineConfig{
		BufferSize: 10,
	})

	handler := message.NewHandler[TestCommand](func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return nil, nil
	}, message.KebabNaming)
	_ = engine.AddHandler("test", nil, handler)

	input := make(chan *message.Message)
	_, _ = engine.AddInput("input", nil, input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = engine.Start(ctx)

	// Try to add middleware after start
	err := engine.Use(middleware.CorrelationID())
	if err != message.ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}

	close(input)
}

// Test types

type TestCommand struct {
	Input int
}

type TestEvent struct {
	Value int
}
