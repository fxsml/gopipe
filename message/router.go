package message

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/channel"
)

// RouterConfig configures message routing behavior.
type RouterConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *pipe.RetryConfig
	Recover     bool
	Middleware  []MiddlewareFunc
}

// Router dispatches messages to handlers based on attribute matching.
type Router struct {
	mu         sync.Mutex
	handlers   []Handler
	pipes      []Pipe
	generators []Generator
	config     RouterConfig
	started    bool
}

// NewRouter creates a router with the given configuration.
// Use AddHandler to add handlers before calling Start.
func NewRouter(config RouterConfig) *Router {
	return &Router{
		config: config,
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
func (r *Router) AddPipe(pipe Pipe) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return false
	}
	r.pipes = append(r.pipes, pipe)
	return true
}

// AddGenerator adds a generator that produces messages for the router.
// Returns false if the router has already started.
func (r *Router) AddGenerator(generator Generator) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return false
	}
	r.generators = append(r.generators, generator)
	return true
}

// Start processes messages through matched handlers and pipes.
// Can only be called once. Subsequent calls return nil.
// Returns nil if msgs is nil and no generators are configured.
func (r *Router) Start(ctx context.Context, msgs <-chan *Message) <-chan *Message {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil
	}
	r.started = true
	// Take a snapshot of handlers, pipes, and generators under lock
	handlers := r.handlers
	pipes := r.pipes
	generators := r.generators
	r.mu.Unlock()

	// Return nil if no input and no generators
	if msgs == nil && len(generators) == 0 {
		return nil
	}

	// Start all generators and merge with input
	var allInputs []<-chan *Message
	if msgs != nil {
		allInputs = append(allInputs, msgs)
	}
	for _, gen := range generators {
		allInputs = append(allInputs, gen.Generate(ctx))
	}

	// Merge all inputs into a single channel
	var mergedInput <-chan *Message
	if len(allInputs) == 1 {
		mergedInput = allInputs[0]
	} else {
		mergedInput = channel.Merge(allInputs...)
	}

	// If no pipes, use simple handler-based routing
	if len(pipes) == 0 {
		return r.startWithHandlers(ctx, mergedInput, handlers)
	}

	// Start all pipes and create their input channels
	pipeInputs := make([]chan *Message, len(pipes))
	pipeOutputs := make([]<-chan *Message, len(pipes))

	for i, pe := range pipes {
		pipeInputs[i] = make(chan *Message)
		pipeOutputs[i] = pe.Start(ctx, pipeInputs[i])
	}

	// Create handler input channel
	handlerInput := make(chan *Message)
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

		for msg := range mergedInput {
			// Check if message matches any pipe
			matched := false
			for i, pe := range pipes {
				if pe.Match(msg.Attributes) {
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
func (r *Router) startWithHandlers(ctx context.Context, msgs <-chan *Message, handlers []Handler) <-chan *Message {
	handle := func(ctx context.Context, msg *Message) ([]*Message, error) {
		for _, h := range handlers {
			if h.Match(msg.Attributes) {
				return h.Handle(ctx, msg)
			}
		}
		err := fmt.Errorf("no handler matched")
		msg.Nack(err)
		return nil, err
	}

	opts := []pipe.Option[*Message, *Message]{
		pipe.WithLogConfig[*Message, *Message](pipe.LogConfig{
			MessageSuccess: "Processed messages",
			MessageFailure: "Failed to process messages",
			MessageCancel:  "Canceled processing messages",
		}),
		pipe.WithMetadataProvider[*Message, *Message](func(msg *Message) pipe.Metadata {
			metadata := pipe.Metadata{}
			if id, ok := msg.Attributes.ID(); ok {
				metadata[AttrID] = id
			}
			if corr, ok := msg.Attributes.CorrelationID(); ok {
				metadata[AttrCorrelationID] = corr
			}

			return metadata
		}),
	}
	if r.config.Recover {
		opts = append(opts, pipe.WithRecover[*Message, *Message]())
	}
	if r.config.Concurrency > 0 {
		opts = append(opts, pipe.WithConcurrency[*Message, *Message](r.config.Concurrency))
	}
	if r.config.Timeout > 0 {
		opts = append(opts, pipe.WithTimeout[*Message, *Message](r.config.Timeout))
	}
	if r.config.Retry != nil {
		opts = append(opts, pipe.WithRetryConfig[*Message, *Message](*r.config.Retry))
	}
	for _, m := range r.config.Middleware {
		opts = append(opts, pipe.WithMiddleware(m))
	}

	return pipe.NewProcessPipe(handle, opts...).Start(ctx, msgs)
}
