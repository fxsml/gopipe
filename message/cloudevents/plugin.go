package cloudevents

import (
	"context"
	"fmt"

	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/fxsml/gopipe/message"
)

// SubscriberPlugin creates a plugin that adds a CloudEvents Receiver as an input source.
// Wraps the receiver with a Subscriber, starts receiving, and registers the
// channel with the engine.
func SubscriberPlugin(ctx context.Context, name string, matcher message.Matcher,
	receiver protocol.Receiver, cfg SubscriberConfig) message.Plugin {
	return func(e *message.Engine) error {
		sub := NewSubscriber(receiver, cfg)
		ch, err := sub.Subscribe(ctx)
		if err != nil {
			return fmt.Errorf("cloudevents subscriber %s: %w", name, err)
		}
		_, err = e.AddRawInput(name, matcher, ch)
		if err != nil {
			return fmt.Errorf("cloudevents subscriber %s: %w", name, err)
		}
		return nil
	}
}

// PublisherPlugin creates a plugin that adds a CloudEvents Sender as an output sink.
// Wraps the sender with a Publisher, registers an output channel with the
// engine, and starts publishing.
func PublisherPlugin(ctx context.Context, name string, matcher message.Matcher,
	sender protocol.Sender, cfg PublisherConfig) message.Plugin {
	return func(e *message.Engine) error {
		pub := NewPublisher(sender, cfg)
		ch, err := e.AddRawOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("cloudevents publisher %s: %w", name, err)
		}
		_, err = pub.Publish(ctx, ch)
		if err != nil {
			return fmt.Errorf("cloudevents publisher %s: %w", name, err)
		}
		return nil
	}
}
