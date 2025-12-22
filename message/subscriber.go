package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// Subscriber provides channel-based message consumption from a Receiver.
type Subscriber struct {
	receiver Receiver
	config   SubscriberConfig
}

// Subscribe creates a channel that polls the specified topic and emits received messages.
// The channel is closed when the context is canceled.
//
// Subscribe can be called multiple times with the same or different topics. Each call
// creates an independent subscription with its own polling goroutine and message channel.
// The behavior of subscribing to the same topic multiple times depends on the underlying
// Receiver implementation - the Subscriber does not prevent or deduplicate multiple
// subscriptions to the same topic.
func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *Message, error) {
	handle := func(ctx context.Context) ([]*Message, error) {
		return s.receiver.Receive(ctx, topic)
	}

	// Build middleware chain
	var mw []middleware.Middleware[struct{}, *Message]

	mw = append(mw, middleware.Log[struct{}, *Message](middleware.LogConfig{
		MessageSuccess: "Received messages",
		MessageFailure: "Failed to receive messages",
		MessageCancel:  "Canceled receiving messages",
	}))

	if s.config.Retry != nil {
		mw = append(mw, middleware.Retry[struct{}, *Message](*s.config.Retry))
	}
	if s.config.Timeout > 0 {
		mw = append(mw, middleware.Context[struct{}, *Message](middleware.ContextConfig{
			Timeout:    s.config.Timeout,
			Background: true,
		}))
	}
	if s.config.Recover {
		mw = append(mw, middleware.Recover[struct{}, *Message]())
	}

	cfg := pipe.Config{
		Concurrency: s.config.Concurrency,
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}

	gen := pipe.NewGenerator(handle, cfg)
	if err := gen.ApplyMiddleware(mw...); err != nil {
		return nil, err
	}
	return gen.Generate(ctx)
}

// SubscriberConfig configures the Subscriber behavior.
type SubscriberConfig struct {
	// Concurrency is the number of concurrent receive operations per topic. Default: 1.
	Concurrency int
	// Timeout is the maximum duration for each receive operation.
	Timeout time.Duration
	// Retry configures automatic retry on failures.
	Retry *middleware.RetryConfig
	// Recover enables panic recovery in receive operations.
	Recover bool
}

// NewSubscriber creates a Subscriber that wraps a Receiver with gopipe processing.
// The subscriber polls the receiver for messages and emits them on a channel.
// Each call to Subscribe creates an independent subscription with its own polling goroutine.
//
// Note: Multiple subscriptions to the same topic are allowed. The behavior depends on the
// underlying Receiver implementation - some may support competing consumers while others
// may deliver duplicate messages.
//
// Panics if receiver is nil.
func NewSubscriber(
	receiver Receiver,
	config SubscriberConfig,
) *Subscriber {
	if receiver == nil {
		panic("message: receiver cannot be nil")
	}

	return &Subscriber{
		receiver: receiver,
		config:   config,
	}
}
