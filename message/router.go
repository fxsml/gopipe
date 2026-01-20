package message

import (
	"context"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
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
	BufferSize   int          // Output channel buffer size (default: 100)
	Concurrency  int          // Number of concurrent handler invocations (default: 1)
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
		"component", "router",
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
		r.logger.Info("Using middleware", "component", "router", "middleware", funcName(mw))
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
	entry, ok := r.handler(msg.Type())
	if !ok {
		err := ErrNoHandler
		msg.Nack(err)
		r.logger.Error("Routing message failed",
			"component", "router",
			"error", err,
			"attributes", msg.Attributes)
		return nil, err
	}

	if entry.matcher != nil && !entry.matcher.Match(msg.Attributes) {
		err := ErrHandlerRejected
		msg.Nack(err)
		r.logger.Error("Matching handler failed",
			"component", "router",
			"handler", entry.name,
			"error", err,
			"attributes", msg.Attributes)
		return nil, err
	}

	outputs, err := entry.handler.Handle(ctx, msg)
	if err != nil {
		msg.Nack(err)
		r.logger.Error("Executing handler failed",
			"component", "router",
			"handler", entry.name,
			"error", err,
			"attributes", msg.Attributes)
		return nil, err
	}

	msg.Ack()
	r.logger.Debug("Message handled successfully",
		"component", "router",
		"handler", entry.name,
		"attributes", msg.Attributes)
	return outputs, nil
}

// Verify Router implements InputRegistry.
var _ InputRegistry = (*Router)(nil)

// funcName extracts a readable name from a function.
// For package-level functions, returns "package.Function" (e.g., "context.Background").
// For closures, returns the factory function name (e.g., "factory" from "factory.func1").
func funcName(f any) string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()

	// Strip generic type parameters [...] to handle closures from generic functions.
	// Example: "pkg.GenericFunc[...].func1" -> "pkg.GenericFunc.func1"
	if idx := strings.Index(name, "["); idx >= 0 {
		if end := strings.LastIndex(name, "]"); end > idx {
			name = name[:idx] + name[end+1:]
		}
	}

	// Find package boundary (after last /)
	// "github.com/user/repo/pkg.Func" -> "pkg.Func"
	pkgPart := name
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		pkgPart = name[slash+1:]
	}

	if dot := strings.LastIndex(pkgPart, "."); dot >= 0 {
		suffix := pkgPart[dot+1:]
		if isClosureSuffix(suffix) {
			// Closure: traverse up past any intermediate funcN to find the factory name
			parent := pkgPart[:dot]
			for {
				dot2 := strings.LastIndex(parent, ".")
				if dot2 < 0 {
					// Reached the end; if still a closure suffix, return "custom"
					if isClosureSuffix(parent) {
						return "custom"
					}
					return parent
				}
				segment := parent[dot2+1:]
				if !isClosureSuffix(segment) {
					return segment
				}
				parent = parent[:dot2]
			}
		}
		// Package-level function: return package.FunctionName
		return pkgPart
	}
	return "custom"
}

// isClosureSuffix checks if a name segment is a closure indicator.
// Go names closures as "func1", "func2", etc. or just numeric like "1", "2".
func isClosureSuffix(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] >= '0' && s[0] <= '9' {
		return true
	}
	return len(s) > 4 && s[:4] == "func" && s[4] >= '0' && s[4] <= '9'
}
