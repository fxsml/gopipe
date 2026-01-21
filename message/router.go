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
	pool    string
}

// PoolConfig configures a worker pool.
type PoolConfig struct {
	// Workers is the number of concurrent workers (default: 1).
	Workers int
	// BufferSize is the output channel buffer (0 = inherit from RouterConfig).
	BufferSize int
}

func (c PoolConfig) parse(defaultBufferSize int) PoolConfig {
	if c.Workers <= 0 {
		c.Workers = 1
	}
	if c.BufferSize <= 0 {
		c.BufferSize = defaultBufferSize
	}
	return c
}

// poolEntry holds pool configuration.
type poolEntry struct {
	cfg PoolConfig
}

// Router dispatches messages to handlers by CE type.
// Implements Pipe signature for composability with pipe.Apply().
// Uses pipe.ProcessPipe internally for middleware, concurrency, and error handling.
// Supports named pools for per-handler concurrency control.
type Router struct {
	logger Logger

	mu           sync.RWMutex
	handlers     map[string]handlerEntry
	pools        map[string]poolEntry
	errorHandler ErrorHandler
	bufferSize   int
	middleware   []Middleware
	started      bool
}

// RouterConfig configures the message router.
type RouterConfig struct {
	// BufferSize is the output channel buffer size (default: 100).
	BufferSize int
	// Pool configures the default pool (default: 1 worker).
	Pool PoolConfig
	// ErrorHandler is called on processing errors (default: no-op, errors logged via Logger).
	ErrorHandler ErrorHandler
	// Logger for router events (default: slog.Default()).
	Logger Logger
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
	c.Pool = c.Pool.parse(c.BufferSize)
	return c
}

// NewRouter creates a new message router.
func NewRouter(cfg RouterConfig) *Router {
	cfg = cfg.parse()
	r := &Router{
		handlers:     make(map[string]handlerEntry),
		pools:        make(map[string]poolEntry),
		bufferSize:   cfg.BufferSize,
		errorHandler: cfg.ErrorHandler,
		logger:       cfg.Logger,
	}
	// Create default pool from config (cannot fail: valid name, not started, no duplicates)
	_ = r.AddPoolWithConfig("default", cfg.Pool)
	return r
}

// AddPoolWithConfig creates a named worker pool.
// Returns error if name is empty, pool already exists, or router already started.
func (r *Router) AddPoolWithConfig(name string, cfg PoolConfig) error {
	if name == "" {
		return ErrPoolNameEmpty
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrAlreadyStarted
	}
	if _, exists := r.pools[name]; exists {
		return ErrPoolExists
	}
	parsed := cfg.parse(r.bufferSize)
	r.pools[name] = poolEntry{cfg: parsed}
	r.logger.Info("Adding pool",
		"component", "router",
		"pool", name,
		"workers", parsed.Workers)
	return nil
}

// AddHandler registers a handler to the default pool.
// The optional matcher is applied after type matching.
func (r *Router) AddHandler(name string, matcher Matcher, h Handler) error {
	return r.AddHandlerToPool(name, matcher, h, "default")
}

// AddHandlerToPool registers a handler to a named pool.
// Returns error if pool doesn't exist, handler already registered, or router already started.
func (r *Router) AddHandlerToPool(name string, matcher Matcher, h Handler, pool string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrAlreadyStarted
	}
	if _, exists := r.pools[pool]; !exists {
		return ErrPoolNotFound
	}
	eventType := h.EventType()
	if _, exists := r.handlers[eventType]; exists {
		return ErrHandlerExists
	}
	r.handlers[eventType] = handlerEntry{name: name, matcher: matcher, handler: h, pool: pool}
	r.logger.Info("Adding handler",
		"component", "router",
		"handler", name,
		"event_type", eventType,
		"pool", pool)
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
		r.logger.Info("Using middleware",
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

	// Apply middleware: first registered wraps outermost
	fn := r.process
	for i := len(r.middleware) - 1; i >= 0; i-- {
		fn = r.middleware[i](fn)
	}

	// Check if we have multiple pools
	hasMultiplePools := len(r.pools) > 1
	r.mu.Unlock()

	// For single pool (common case), use simple ProcessPipe for best performance
	if !hasMultiplePools {
		return r.pipeSinglePool(ctx, in, fn)
	}

	// For multiple pools, use Distributor + ProcessPipes + Merger
	return r.pipeMultiPool(ctx, in, fn)
}

// pipeSinglePool handles the common single-pool case with a simple ProcessPipe.
func (r *Router) pipeSinglePool(ctx context.Context, in <-chan *Message, fn ProcessFunc) (<-chan *Message, error) {
	r.mu.RLock()
	defaultPool := r.pools["default"]
	r.mu.RUnlock()

	cfg := pipe.Config{
		BufferSize:  defaultPool.cfg.BufferSize,
		Concurrency: defaultPool.cfg.Workers,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			r.errorHandler(msg, err)
		},
	}
	p := pipe.NewProcessPipe(fn, cfg)
	return p.Pipe(ctx, in)
}

// pipeMultiPool handles multiple pools using Distributor + ProcessPipes + Merger.
func (r *Router) pipeMultiPool(ctx context.Context, in <-chan *Message, fn ProcessFunc) (<-chan *Message, error) {
	// in → Distributor(makePoolMatcher) → ProcessPipe(workers) → Merger → out

	r.mu.RLock()
	pools := make(map[string]poolEntry, len(r.pools))
	for name, pool := range r.pools {
		pools[name] = pool
	}
	r.mu.RUnlock()

	dist := pipe.NewDistributor[*Message](pipe.DistributorConfig[*Message]{
		Buffer: r.bufferSize,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			r.logger.Error("Routing message failed",
				"component", "router",
				"error", err,
				"attributes", msg.Attributes)
			r.errorHandler(msg, err)
		},
	})

	merger := pipe.NewMerger[*Message](pipe.MergerConfig{
		Buffer: r.bufferSize,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			r.logger.Warn("Message merge failed",
				"component", "router",
				"error", err,
				"attributes", msg.Attributes)
			r.errorHandler(msg, err)
		},
	})

	// Wire: Distributor output(poolMatcher) → ProcessPipe(workers) → Merger input
	for poolName, pool := range pools {
		poolIn, err := dist.AddOutput(r.makePoolMatcher(poolName))
		if err != nil {
			return nil, err
		}

		cfg := pipe.Config{
			BufferSize:  pool.cfg.BufferSize,
			Concurrency: pool.cfg.Workers,
			ErrorHandler: func(in any, err error) {
				msg := in.(*Message)
				r.errorHandler(msg, err)
			},
		}
		poolPipe := pipe.NewProcessPipe(fn, cfg)

		poolOut, err := poolPipe.Pipe(ctx, poolIn)
		if err != nil {
			return nil, err
		}
		if _, err := merger.AddInput(poolOut); err != nil {
			return nil, err
		}
	}

	if _, err := dist.Distribute(ctx, in); err != nil {
		return nil, err
	}
	return merger.Merge(ctx)
}

// makePoolMatcher returns a Distributor matcher for event types in this pool.
func (r *Router) makePoolMatcher(poolName string) func(*Message) bool {
	return func(msg *Message) bool {
		// lookup handler by event type, check if it belongs to this pool
		entry, ok := r.handler(msg.Type())
		return ok && entry.pool == poolName
	}
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
	// handler lookup → matcher check → handler.Handle → ack/nack
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
