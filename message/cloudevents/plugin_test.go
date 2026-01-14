package cloudevents

import (
	"context"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/fxsml/gopipe/message"
)

// TestEvent is the typed event for testing.
type TestEvent struct {
	Value string `json:"value"`
}

func TestSubscriberPlugin(t *testing.T) {
	t.Run("registers input and receives messages", func(t *testing.T) {
		event := cloudevents.NewEvent()
		event.SetID("plugin-test")
		event.SetType("test.event")
		event.SetSource("/test")
		if err := event.SetData("application/json", TestEvent{Value: "hello"}); err != nil {
			t.Fatalf("failed to set data: %v", err)
		}

		receiver := newMockReceiver(&event)

		engine := message.NewEngine(message.EngineConfig{
			Marshaler: message.NewJSONMarshaler(),
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		// Add handler BEFORE plugin (plugin starts receiving immediately)
		received := make(chan string, 1)
		handler := message.NewCommandHandler(
			func(ctx context.Context, data TestEvent) ([]struct{}, error) {
				received <- data.Value
				return nil, nil
			},
			message.CommandHandlerConfig{Source: "/test", Naming: message.KebabNaming},
		)
		if err := engine.AddHandler("test-handler", nil, handler); err != nil {
			t.Fatalf("failed to add handler: %v", err)
		}

		err := engine.AddPlugin(
			SubscriberPlugin(ctx, "test-input", nil, receiver, SubscriberConfig{}),
		)
		if err != nil {
			t.Fatalf("failed to add plugin: %v", err)
		}

		done, err := engine.Start(ctx)
		if err != nil {
			t.Fatalf("failed to start engine: %v", err)
		}

		select {
		case v := <-received:
			if v != "hello" {
				t.Errorf("expected 'hello', got %q", v)
			}
		case <-done:
			t.Error("engine stopped before receiving message")
		case <-ctx.Done():
			t.Error("timeout waiting for message")
		}
	})
}

// OutputEvent is the output event for testing publisher.
type OutputEvent struct {
	Result string `json:"result"`
}

func TestPublisherPlugin(t *testing.T) {
	t.Run("registers output and sends messages", func(t *testing.T) {
		sender := newMockSender(protocol.ResultACK)

		engine := message.NewEngine(message.EngineConfig{
			Marshaler: message.NewJSONMarshaler(),
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := engine.AddPlugin(
			PublisherPlugin(ctx, "test-output", nil, sender, PublisherConfig{}),
		)
		if err != nil {
			t.Fatalf("failed to add plugin: %v", err)
		}

		// Add handler that produces output
		handler := message.NewCommandHandler(
			func(ctx context.Context, data TestEvent) ([]OutputEvent, error) {
				return []OutputEvent{{Result: "done"}}, nil
			},
			message.CommandHandlerConfig{Source: "/test", Naming: message.KebabNaming},
		)
		if err := engine.AddHandler("test-handler", nil, handler); err != nil {
			t.Fatalf("failed to add handler: %v", err)
		}

		// Add typed input
		in := make(chan *message.Message, 1)
		if _, err := engine.AddInput("test-input", nil, in); err != nil {
			t.Fatalf("failed to add input: %v", err)
		}

		done, err := engine.Start(ctx)
		if err != nil {
			t.Fatalf("failed to start engine: %v", err)
		}

		// Send input message
		in <- message.New(
			TestEvent{Value: "test"},
			message.Attributes{
				"id":     "in-test",
				"type":   "test.event",
				"source": "/test",
			},
			nil,
		)
		close(in)

		<-done

		if sender.MessageCount() != 1 {
			t.Errorf("expected 1 message sent, got %d", sender.MessageCount())
		}
	})
}
