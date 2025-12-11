package cqrs

import (
	"context"
	"fmt"
	"sync"
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

// Router dispatches messages to handlers based on attribute matching.
type Router struct {
	mu       sync.Mutex
	handlers []Handler
	pipes    []pipeEntry
	config   RouterConfig
	started  bool
}

type pipeEntry struct {
	pipe  gopipe.Pipe[*message.Message, *message.Message]
	match func(attrs message.Attributes) bool
}

// NewRouter creates a router with the given configuration and handlers.
func NewRouter(config RouterConfig, handlers ...Handler) *Router {
	return &Router{
		handlers: handlers,
		config:   config,
	}
}

// AddHandler adds a handler to the router.
// Returns false if the router has already started.
func (r *Router) AddHandler(handler Handler) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return false
	}
	r.handlers = append(r.handlers, handler)
	return true
}

// AddPipe adds a pipe that receives matching messages.
// Returns false if the router has already started.
func (r *Router) AddPipe(pipe gopipe.Pipe[*message.Message, *message.Message], match func(attrs message.Attributes) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return false
	}
	r.pipes = append(r.pipes, pipeEntry{
		pipe:  pipe,
		match: match,
	})
	return true
}

// Start processes messages through matched handlers and pipes.
// Can only be called once. Subsequent calls return nil.
func (r *Router) Start(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil
	}
	r.started = true
	// Take a snapshot of handlers and pipes under lock
	handlers := r.handlers
	pipes := r.pipes
	r.mu.Unlock()

	// If no pipes, use simple handler-based routing
	if len(pipes) == 0 {
		return r.startWithHandlers(ctx, msgs, handlers)
	}

	// Start all pipes and create their input channels
	pipeInputs := make([]chan *message.Message, len(pipes))
	pipeOutputs := make([]<-chan *message.Message, len(pipes))

	for i, pe := range pipes {
		pipeInputs[i] = make(chan *message.Message)
		pipeOutputs[i] = pe.pipe.Start(ctx, pipeInputs[i])
	}

	// Create handler input channel
	handlerInput := make(chan *message.Message)
	handlerOutput := r.startWithHandlers(ctx, handlerInput, handlers)

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
			for i, pe := range pipes {
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

// startWithHandlers processes messages through the given handlers.
func (r *Router) startWithHandlers(ctx context.Context, msgs <-chan *message.Message, handlers []Handler) <-chan *message.Message {
	handle := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		for _, h := range handlers {
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
