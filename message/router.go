package message

import (
	"context"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

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

// PoolConfig configures concurrency and buffering for process pipes.
type PoolConfig struct {
	// Workers is the number of concurrent workers (default: 1).
	Workers int
	// BufferSize is the output channel buffer size (default: 100).
	BufferSize int
}

func (c PoolConfig) parse() PoolConfig {
	if c.Workers <= 0 {
		c.Workers = 1
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	return c
}

// PipeConfig configures process pipes (Router, UnmarshalPipe, MarshalPipe).
type PipeConfig struct {
	// Pool configures concurrency and buffering (default: 1 worker, 100 buffer).
	Pool PoolConfig
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	ShutdownTimeout time.Duration
	// AckStrategy determines how messages are acknowledged (default: AckManual).
	// AckManual: handler responsible; AckOnSuccess: auto-ack; AckForward: ack when outputs ack.
	AckStrategy AckStrategy
	// Logger for pipe events (default: slog.Default()).
	Logger Logger
	// ErrorHandler is called on processing errors (default: no-op, errors logged via Logger).
	ErrorHandler ErrorHandler
}

func (c PipeConfig) parse() PipeConfig {
	c.Pool = c.Pool.parse()
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(msg *Message, err error) {}
	}
	return c
}

// Router dispatches messages to handlers by CE type.
// Implements Pipe signature for composability with pipe.Apply().
// Uses pipe.ProcessPipe internally for middleware, concurrency, and error handling.
type Router struct {
	cfg PipeConfig

	mu         sync.RWMutex
	handlers   map[string]handlerEntry
	middleware []Middleware
	started    bool
}

// NewRouter creates a new message router.
func NewRouter(cfg PipeConfig) *Router {
	cfg = cfg.parse()
	r := &Router{
		cfg:      cfg,
		handlers: make(map[string]handlerEntry),
	}
	r.cfg.Logger.Info("Adding pool",
		"component", "router",
		"pool", "default",
		"workers", cfg.Pool.Workers,
		"ack_strategy", ackStrategyName(cfg.AckStrategy))
	return r
}

// ackStrategyName returns a human-readable name for the strategy.
func ackStrategyName(s AckStrategy) string {
	switch s {
	case AckOnSuccess:
		return "on_success"
	case AckForward:
		return "forward"
	default:
		return "manual"
	}
}

// AddHandler registers a handler.
// The optional matcher is applied after type matching.
func (r *Router) AddHandler(name string, matcher Matcher, h Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrAlreadyStarted
	}
	eventType := h.EventType()
	if _, exists := r.handlers[eventType]; exists {
		return ErrHandlerExists
	}
	r.handlers[eventType] = handlerEntry{name: name, matcher: matcher, handler: h}
	r.cfg.Logger.Info("Adding handler",
		"component", "router",
		"handler", name,
		"event_type", eventType,
		"pool", "default")
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
		r.cfg.Logger.Info("Using middleware",
			"component", "router",
			"middleware", funcName(mw))
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

	// Apply acking strategy as innermost middleware (closest to handler)
	fn := r.process
	if ackMw := r.ackingMiddleware(); ackMw != nil {
		fn = ackMw(fn)
	}

	// Apply user middleware: first registered wraps outermost
	for i := len(r.middleware) - 1; i >= 0; i-- {
		fn = r.middleware[i](fn)
	}
	r.mu.Unlock()

	cfg := pipe.Config{
		BufferSize:      r.cfg.Pool.BufferSize,
		Concurrency:     r.cfg.Pool.Workers,
		ShutdownTimeout: r.cfg.ShutdownTimeout,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			r.cfg.ErrorHandler(msg, err)
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
	// handler lookup → matcher check → handler.Handle
	// Messages are auto-nacked on error (consistent with other components).
	// Acking on success is the handler's responsibility. Use AutoAck middleware
	// for automatic ack-on-success behavior.
	entry, ok := r.handler(msg.Type())
	if !ok {
		err := ErrNoHandler
		r.cfg.Logger.Error("Routing message failed",
			"component", "router",
			"error", err,
			"attributes", msg.Attributes)
		return nil, err
	}

	if entry.matcher != nil && !entry.matcher.Match(msg.Attributes) {
		err := ErrHandlerRejected
		r.cfg.Logger.Error("Matching handler failed",
			"component", "router",
			"handler", entry.name,
			"error", err,
			"attributes", msg.Attributes)
		return nil, err
	}

	outputs, err := entry.handler.Handle(msg.Context(ctx), msg)
	if err != nil {
		r.cfg.Logger.Error("Executing handler failed",
			"component", "router",
			"handler", entry.name,
			"error", err,
			"attributes", msg.Attributes)
		return nil, err
	}

	r.cfg.Logger.Debug("Message handled successfully",
		"component", "router",
		"handler", entry.name,
		"attributes", msg.Attributes)
	return outputs, nil
}

// ackingMiddleware returns the middleware for the configured ack strategy.
// Returns nil for AckManual (no automatic acking).
func (r *Router) ackingMiddleware() Middleware {
	switch r.cfg.AckStrategy {
	case AckOnSuccess:
		return autoAckMiddleware()
	case AckForward:
		return forwardAckMiddleware()
	default:
		return nil
	}
}

// autoAckMiddleware returns middleware that acks on success, nacks on error.
func autoAckMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}
			msg.Ack()
			return outputs, nil
		}
	}
}

// forwardAckMiddleware returns middleware that forwards ack to output messages.
func forwardAckMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}

			// No outputs - ack input immediately
			if len(outputs) == 0 {
				msg.Ack()
				return outputs, nil
			}

			// Check if input was already acked (handler acked manually)
			if msg.AckState() != AckPending {
				return outputs, nil
			}

			// Create shared acking: input acked when all outputs acked
			shared := NewSharedAcking(
				func() { msg.Ack() },
				func(e error) { msg.Nack(e) },
				len(outputs),
			)

			// Replace each output's acking with the shared acking
			for _, out := range outputs {
				out.Acking = shared
			}

			return outputs, nil
		}
	}
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
