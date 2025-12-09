package cqrs

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

var (
	// ErrInvalidMessagePayload indicates message payload marshal or unmarshal failure.
	ErrInvalidMessagePayload = fmt.Errorf("invalid message payload")
)

// RouterConfig configures message routing behavior.
type RouterConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *gopipe.RetryConfig
	Recover     bool
	Middleware  []gopipe.MiddlewareFunc[*message.Message, *message.Message]
}

// Router dispatches messages to registered handlers based on property matching.
type Router struct {
	handlers []Handler
	config   RouterConfig
}

// NewRouter creates a router with the given configuration and handlers.
func NewRouter(config RouterConfig, handlers ...Handler) *Router {
	return &Router{
		handlers: handlers,
		config:   config,
	}
}

// AddHandler appends a handler to the router's handler list.
func (r *Router) AddHandler(handler Handler) {
	r.handlers = append(r.handlers, handler)
}

// Start processes incoming messages through matched handlers and returns output messages.
func (r *Router) Start(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	handle := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		for _, h := range r.handlers {
			if h.Match(msg.Properties) {
				return h.Handle(ctx, msg)
			}
		}
		err := fmt.Errorf("no handler matched")
		msg.Nack(err)
		return nil, err
	}

	opts := []gopipe.Option[*message.Message, *message.Message]{
		gopipe.WithLogConfig[*message.Message, *message.Message](gopipe.LogConfig{
			MessageSuccess: "Processed messages",
			MessageFailure: "Failed to process messages",
			MessageCancel:  "Canceled processing messages",
		}),
		gopipe.WithMetadataProvider[*message.Message, *message.Message](func(msg *message.Message) gopipe.Metadata {
			metadata := gopipe.Metadata{}
			if id, ok := msg.Properties.ID(); ok {
				metadata[message.PropID] = id
			}
			if corr, ok := msg.Properties.CorrelationID(); ok {
				metadata[message.PropCorrelationID] = corr
			}
			return metadata
		}),
	}
	if r.config.Recover {
		opts = append(opts, gopipe.WithRecover[*message.Message, *message.Message]())
	}
	if r.config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[*message.Message, *message.Message](r.config.Concurrency))
	}
	if r.config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[*message.Message, *message.Message](r.config.Timeout))
	}
	if r.config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[*message.Message, *message.Message](*r.config.Retry))
	}
	for _, m := range r.config.Middleware {
		opts = append(opts, gopipe.WithMiddleware(m))
	}

	return gopipe.NewProcessPipe(handle, opts...).Start(ctx, msgs)
}

// Handler processes messages matching specific properties.
type Handler interface {
	Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error)
	Match(prop message.Properties) bool
}

type handler struct {
	handle func(ctx context.Context, msg *message.Message) ([]*message.Message, error)
	match  func(prop message.Properties) bool
}

func (h *handler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	return h.handle(ctx, msg)
}

func (h *handler) Match(prop message.Properties) bool {
	return h.match(prop)
}

// NewHandler creates a handler from message processing and matching functions.
func NewHandler(
	handle func(ctx context.Context, msg *message.Message) ([]*message.Message, error),
	match func(prop message.Properties) bool,
) Handler {
	return &handler{
		handle: handle,
		match:  match,
	}
}
