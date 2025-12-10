package cqrs

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
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
	pipes    []pipeEntry
	config   RouterConfig
}

// pipeEntry stores a pipe with its matcher function.
type pipeEntry struct {
	pipe  gopipe.Pipe[*message.Message, *message.Message]
	match func(prop message.Attributes) bool
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

// AddPipe appends a pipe to the router's pipe list.
// Messages matching the matcher will be sent to the pipe instead of handlers.
func (r *Router) AddPipe(pipe gopipe.Pipe[*message.Message, *message.Message], match func(prop message.Attributes) bool) {
	r.pipes = append(r.pipes, pipeEntry{
		pipe:  pipe,
		match: match,
	})
}

// Start processes incoming messages through matched handlers and returns output messages.
// Messages are routed to pipes first (if any match), then to handlers.
func (r *Router) Start(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	// If no pipes, use simple handler-based routing
	if len(r.pipes) == 0 {
		return r.startWithHandlersOnly(ctx, msgs)
	}

	// Start all pipes and create their input channels
	pipeInputs := make([]chan *message.Message, len(r.pipes))
	pipeOutputs := make([]<-chan *message.Message, len(r.pipes))

	for i, pe := range r.pipes {
		pipeInputs[i] = make(chan *message.Message)
		pipeOutputs[i] = pe.pipe.Start(ctx, pipeInputs[i])
	}

	// Create handler input channel
	handlerInput := make(chan *message.Message)
	handlerOutput := r.startWithHandlersOnly(ctx, handlerInput)

	// Route incoming messages to pipes or handlers
	go func() {
		defer func() {
			// Close all pipe inputs
			for _, in := range pipeInputs {
				close(in)
			}
			close(handlerInput)
		}()

		for msg := range msgs {
			// Check if message matches any pipe
			matched := false
			for i, pe := range r.pipes {
				if pe.match(msg.Attributes) {
					pipeInputs[i] <- msg
					matched = true
					break
				}
			}

			// If no pipe matched, send to handlers
			if !matched {
				handlerInput <- msg
			}
		}
	}()

	// Merge all outputs
	allOutputs := append(pipeOutputs, handlerOutput)
	return channel.Merge(allOutputs...)
}

// startWithHandlersOnly processes messages through handlers only (no pipes).
func (r *Router) startWithHandlersOnly(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	handle := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		for _, h := range r.handlers {
			if h.Match(msg.Attributes) {
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
			if id, ok := msg.Attributes.ID(); ok {
				metadata[message.AttrID] = id
			}
			if corr, ok := msg.Attributes.CorrelationID(); ok {
				metadata[message.AttrCorrelationID] = corr
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
