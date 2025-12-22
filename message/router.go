package message

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// RouterConfig configures message routing behavior.
type RouterConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *middleware.RetryConfig
	Recover     bool
	Middleware  []Middleware
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
// Returns ErrAlreadyStarted if called more than once.
// Returns (nil, nil) if msgs is nil and no generators are configured.
func (r *Router) Start(ctx context.Context, msgs <-chan *Message) (<-chan *Message, error) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	r.started = true
	// Take a snapshot of handlers, pipes, and generators under lock
	handlers := r.handlers
	pipes := r.pipes
	generators := r.generators
	r.mu.Unlock()

	// Return nil if no input and no generators
	if msgs == nil && len(generators) == 0 {
		return nil, nil
	}

	// Start all generators and merge with input
	var allInputs []<-chan *Message
	if msgs != nil {
		allInputs = append(allInputs, msgs)
	}
	for _, gen := range generators {
		out, err := gen.Generate(ctx)
		if err != nil {
			continue // skip failed generators
		}
		allInputs = append(allInputs, out)
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
		return r.startWithHandlers(ctx, mergedInput, handlers), nil
	}

	// Start all pipes and create their input channels
	pipeInputs := make([]chan *Message, len(pipes))
	pipeOutputs := make([]<-chan *Message, len(pipes))

	for i, pe := range pipes {
		pipeInputs[i] = make(chan *Message)
		out, err := pe.Start(ctx, pipeInputs[i])
		if err != nil {
			close(pipeInputs[i])
			continue
		}
		pipeOutputs[i] = out
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
	return channel.Merge(allOutputs...), nil
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

	// Build middleware chain
	var mw []middleware.Middleware[*Message, *Message]

	// Metadata provider first (outermost)
	mw = append(mw, middleware.MetadataProvider[*Message, *Message](func(msg *Message) middleware.Metadata {
		metadata := middleware.Metadata{}
		if id, ok := msg.Attributes.ID(); ok {
			metadata[AttrID] = id
		}
		if corr, ok := msg.Attributes.CorrelationID(); ok {
			metadata[AttrCorrelationID] = corr
		}
		return metadata
	}))

	// Logging
	mw = append(mw, middleware.Log[*Message, *Message](middleware.LogConfig{
		MessageSuccess: "Processed messages",
		MessageFailure: "Failed to process messages",
		MessageCancel:  "Canceled processing messages",
	}))

	// Retry
	if r.config.Retry != nil {
		mw = append(mw, middleware.Retry[*Message, *Message](*r.config.Retry))
	}

	// Timeout
	if r.config.Timeout > 0 {
		mw = append(mw, middleware.Context[*Message, *Message](middleware.ContextConfig{
			Timeout:    r.config.Timeout,
			Background: true,
		}))
	}

	// User middleware
	mw = append(mw, r.config.Middleware...)

	// Recover (innermost, closest to handler)
	if r.config.Recover {
		mw = append(mw, middleware.Recover[*Message, *Message]())
	}

	cfg := pipe.Config{
		Concurrency: r.config.Concurrency,
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}

	pp := pipe.NewProcessPipe(handle, cfg)
	_ = pp.ApplyMiddleware(mw...) // error only if already started
	out, _ := pp.Start(ctx, msgs) // error only if already started
	return out
}
