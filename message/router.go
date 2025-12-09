package message

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
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
func (r *Router) Start(ctx context.Context, msgs <-chan *Message) <-chan *Message {
	handle := func(ctx context.Context, msg *Message) ([]*Message, error) {
		for _, h := range r.handlers {
			if h.Match(msg.Properties) {
				return h.Handle(ctx, msg)
			}
		}
		err := fmt.Errorf("no handler matched")
		msg.Nack(err)
		return nil, err
	}

	opts := []gopipe.Option[*Message, *Message]{
		gopipe.WithLogConfig[*Message, *Message](gopipe.LogConfig{
			MessageSuccess: "Processed messages",
			MessageFailure: "Failed to process messages",
			MessageCancel:  "Canceled processing messages",
		}),
		gopipe.WithMetadataProvider[*Message, *Message](func(msg *Message) gopipe.Metadata {
			metadata := gopipe.Metadata{}
			if id, ok := msg.Properties.ID(); ok {
				metadata[PropID] = id
			}
			if corr, ok := msg.Properties.CorrelationID(); ok {
				metadata[PropCorrelationID] = corr
			}
			return metadata
		}),
	}
	if r.config.Recover {
		opts = append(opts, gopipe.WithRecover[*Message, *Message]())
	}
	if r.config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[*Message, *Message](r.config.Concurrency))
	}
	if r.config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[*Message, *Message](r.config.Timeout))
	}
	if r.config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[*Message, *Message](*r.config.Retry))
	}

	return gopipe.NewProcessPipe(handle, opts...).Start(ctx, msgs)
}

// Handler processes messages matching specific properties.
type Handler interface {
	Handle(ctx context.Context, msg *Message) ([]*Message, error)
	Match(prop Properties) bool
}

type handler struct {
	handle func(ctx context.Context, msg *Message) ([]*Message, error)
	match  func(prop Properties) bool
}

func (h *handler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	return h.handle(ctx, msg)
}

func (h *handler) Match(prop Properties) bool {
	return h.match(prop)
}

// NewHandler creates a handler from message processing and matching functions.
func NewHandler(
	handle func(ctx context.Context, msg *Message) ([]*Message, error),
	match func(prop Properties) bool,
) Handler {
	return &handler{
		handle: handle,
		match:  match,
	}
}

