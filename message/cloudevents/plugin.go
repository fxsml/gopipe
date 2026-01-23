package cloudevents

import (
	"context"
	"fmt"

	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// SubscriberPlugin creates a plugin that adds a CloudEvents Receiver as an input source.
// Wraps the receiver with a Subscriber, applies optional middleware, starts receiving,
// and registers the channel with the engine.
//
// Middleware is from the pipe/middleware package and wraps the receive function,
// enabling retry logic, circuit breaking, or backoff on errors.
func SubscriberPlugin(
	ctx context.Context,
	name string,
	matcher message.Matcher,
	receiver protocol.Receiver,
	cfg SubscriberConfig,
	mw ...middleware.Middleware[struct{}, *message.RawMessage],
) message.Plugin {
	return func(e *message.Engine) error {
		sub := NewSubscriber(receiver, cfg)
		if err := sub.Use(mw...); err != nil {
			return fmt.Errorf("cloudevents subscriber %s: %w", name, err)
		}
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
// Wraps the sender with a Publisher, applies optional middleware, registers an output
// channel with the engine, and starts publishing.
//
// Middleware is from the pipe/middleware package and wraps the send function,
// enabling retry logic, circuit breaking, or backoff on errors.
func PublisherPlugin(
	ctx context.Context,
	name string,
	matcher message.Matcher,
	sender protocol.Sender,
	cfg PublisherConfig,
	mw ...middleware.Middleware[*message.RawMessage, struct{}],
) message.Plugin {
	return func(e *message.Engine) error {
		pub := NewPublisher(sender, cfg)
		if err := pub.Use(mw...); err != nil {
			return fmt.Errorf("cloudevents publisher %s: %w", name, err)
		}
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
