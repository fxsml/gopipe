package message

import (
	"context"
	"log/slog"
	"sync"

	"github.com/fxsml/gopipe/pipe"
)

// ProcessFunc is the message processing function signature.
type ProcessFunc func(context.Context, *Message) ([]*Message, error)

// Middleware wraps a ProcessFunc to add cross-cutting concerns.
type Middleware func(ProcessFunc) ProcessFunc

// handlerEntry holds a handler and its configuration.
type handlerEntry struct {
	name    string
	matcher Matcher
	handler Handler
}

// Router dispatches messages to handlers by CE type.
// Implements Pipe signature for composability with pipe.Apply().
// Uses pipe.ProcessPipe internally for middleware, concurrency, and error handling.
type Router struct {
	logger Logger

	mu           sync.RWMutex
	handlers     map[string]handlerEntry
	errorHandler ErrorHandler
	bufferSize   int
	concurrency  int
	middleware   []Middleware
	started      bool
}

// RouterConfig configures the message router.
type RouterConfig struct {
	BufferSize   int // Output channel buffer size (default: 100)
	Concurrency  int // Number of concurrent handler invocations (default: 1)
	ErrorHandler ErrorHandler // Default: no-op (errors logged via Logger)
	Logger       Logger       // Default: slog.Default()
}

func (c RouterConfig) parse() RouterConfig {
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(msg *Message, err error) {}
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	return c
}

// NewRouter creates a new message router.
func NewRouter(cfg RouterConfig) *Router {
	cfg = cfg.parse()
	return &Router{
		handlers:     make(map[string]handlerEntry),
		bufferSize:   cfg.BufferSize,
		concurrency:  cfg.Concurrency,
		errorHandler: cfg.ErrorHandler,
		logger:       cfg.Logger,
	}
}

// AddHandler registers a handler for its CE type.
// The optional matcher is applied after type matching.
func (r *Router) AddHandler(name string, matcher Matcher, h Handler) error {
	r.logger.Info("Adding handler",
		"handler", name,
		"event_type", h.EventType())
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.EventType()] = handlerEntry{name: name, matcher: matcher, handler: h}
	return nil
}

// Use registers middleware to wrap message processing.
// Middleware is applied in order: first registered wraps outermost.
// Must be called before Pipe().
func (r *Router) Use(m ...Middleware) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrAlreadyStarted
	}
	for _, mw := range m {
		r.logger.Info("Using middleware", "middleware", funcName(mw))
	}
	r.middleware = append(r.middleware, m...)
	return nil
}

// Pipe routes messages to handlers and returns outputs.
// Signature matches pipe.Pipe[*Message, *Message] for composability.
func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	r.started = true

	// Apply middleware: first registered wraps outermost
	fn := r.process
	for i := len(r.middleware) - 1; i >= 0; i-- {
		fn = r.middleware[i](fn)
	}
	r.mu.Unlock()

	cfg := pipe.Config{
		BufferSize:  r.bufferSize,
		Concurrency: r.concurrency,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			r.logger.Error("Handler error",
				"error", err,
				"attributes", msg.Attributes)
			r.errorHandler(msg, err)
		},
	}
	p := pipe.NewProcessPipe(fn, cfg)
	return p.Pipe(ctx, in)
}

// handler returns the handler entry for the given CE type.
func (r *Router) handler(eventType string) (handlerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.handlers[eventType]
	return entry, ok
}

// NewInput creates a typed instance for unmarshaling.
// Implements InputRegistry.
func (r *Router) NewInput(eventType string) any {
	entry, ok := r.handler(eventType)
	if !ok {
		return nil
	}
	return entry.handler.NewInput()
}

func (r *Router) process(ctx context.Context, msg *Message) ([]*Message, error) {
	eventType, _ := msg.Attributes["type"].(string)

	entry, ok := r.handler(eventType)
	if !ok {
		err := ErrNoHandler
		msg.Nack(err)
		return nil, err
	}

	if entry.matcher != nil && !entry.matcher.Match(msg.Attributes) {
		err := ErrHandlerRejected
		msg.Nack(err)
		return nil, err
	}

	outputs, err := entry.handler.Handle(ctx, msg)
	if err != nil {
		msg.Nack(err)
		return nil, err
	}

	msg.Ack()
	r.logger.Debug("Message handled successfully",
		"handler", entry.name,
		"attributes", msg.Attributes)
	return outputs, nil
}

// Verify Router implements InputRegistry.
var _ InputRegistry = (*Router)(nil)
