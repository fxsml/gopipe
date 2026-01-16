package message

import (
	"context"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe"
)

// ErrorHandler processes engine errors.
type ErrorHandler func(msg *Message, err error)

// Plugin registers handlers, middleware, or other components with an engine.
type Plugin func(*Engine) error

// EngineConfig configures the message engine.
type EngineConfig struct {
	// Marshaler converts between typed data and raw bytes.
	Marshaler Marshaler
	// BufferSize is the channel buffer size (default: 100).
	BufferSize int
	// ErrorHandler is called on processing errors (default: no-op).
	// Errors are logged via Logger; use ErrorHandler for custom handling
	// like metrics, alerting, or recovery logic.
	ErrorHandler ErrorHandler
	// Logger for engine events (default: slog.Default()).
	Logger Logger
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, waits indefinitely for inputs to close naturally.
	// If > 0, waits up to this duration then forces shutdown.
	ShutdownTimeout time.Duration
}

func (c EngineConfig) parse() EngineConfig {
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(msg *Message, err error) {}
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	return c
}

// Engine orchestrates message flow between inputs, handlers, and outputs.
// Uses a single merger for all inputs (typed and unmarshaled raw) and
// a single distributor for all outputs (typed and pre-marshal raw).
type Engine struct {
	cfg    EngineConfig
	router *Router

	mu          sync.Mutex
	started     bool
	merger      *pipe.Merger[*Message]
	distributor *pipe.Distributor[*Message]
	shutdown    chan struct{}
}

// NewEngine creates a new message engine.
func NewEngine(cfg EngineConfig) *Engine {
	cfg = cfg.parse()

	e := &Engine{
		cfg:      cfg,
		shutdown: make(chan struct{}),
		router: NewRouter(RouterConfig{
			ErrorHandler: cfg.ErrorHandler,
			BufferSize:   cfg.BufferSize,
			Logger:       cfg.Logger,
		}),
	}

	// Create merger upfront - AddInput works before Merge()
	// ShutdownTimeout ensures merger closes output on context cancel
	e.merger = pipe.NewMerger[*Message](pipe.MergerConfig{
		Buffer:          cfg.BufferSize,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	// Create distributor upfront - AddOutput works before Distribute()
	// Distributor has no ShutdownTimeout - it waits for input to close naturally,
	// ensuring all messages that pass the merger are delivered to outputs.
	e.distributor = pipe.NewDistributor(pipe.DistributorConfig[*Message]{
		Buffer: cfg.BufferSize,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			e.cfg.Logger.Warn("Distributor error",
				"error", err,
				"attributes", msg.Attributes)
			e.cfg.ErrorHandler(msg, err)
		},
	})

	return e
}

// AddHandler registers a handler for its CE type.
// The optional matcher is applied after type matching.
func (e *Engine) AddHandler(name string, matcher Matcher, h Handler) error {
	return e.router.AddHandler(name, matcher, h)
}

// Use registers middleware to wrap message processing.
// Middleware is applied in order: first registered wraps outermost.
// Must be called before Start().
func (e *Engine) Use(m ...Middleware) error {
	return e.router.Use(m...)
}

// AddPlugin registers plugins that configure the engine.
// Plugins can add handlers, middleware, inputs, outputs, etc.
// Must be called before Start().
//
// Ordering: Plugins that add outputs are matched first-wins. Call AddPlugin
// before AddOutput if the plugin should have priority when matchers overlap.
//
// Loopback plugins create cycles in message flow - the engine cannot detect
// completion. Cancel the context to trigger shutdown.
func (e *Engine) AddPlugin(plugins ...Plugin) error {
	for _, p := range plugins {
		e.cfg.Logger.Info("Adding plugin", "plugin", funcName(p))
		if err := p(e); err != nil {
			return err
		}
	}
	return nil
}

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

// AddInput registers a typed input channel.
// Typed inputs go directly to the merger, bypassing unmarshaling.
// Use for internal messaging, testing, or when data is already typed.
// Can be called before or after Start().
func (e *Engine) AddInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error) {
	e.cfg.Logger.Info("Adding input", "input", name)
	filtered := e.applyTypedInputMatcher(name, ch, matcher)
	return e.merger.AddInput(filtered)
}

// AddRawInput registers a raw input channel.
// Raw inputs are filtered, unmarshaled, and fed to the merger.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawInput(name string, matcher Matcher, ch <-chan *RawMessage) (<-chan struct{}, error) {
	e.cfg.Logger.Info("Adding raw input", "input", name)
	filtered := e.applyRawInputMatcher(name, ch, matcher)
	return e.merger.AddInput(e.unmarshal(filtered))
}

// AddOutput registers a typed output and returns the channel to consume from.
// Typed outputs receive messages directly from the distributor without marshaling.
// Use for internal messaging or when you need typed access to messages.
// Can be called before or after Start().
func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error) {
	e.cfg.Logger.Info("Adding output", "output", name)
	return e.distributor.AddOutput(func(msg *Message) bool {
		return matcher == nil || matcher.Match(msg.Attributes)
	})
}

// AddRawOutput registers a raw output and returns the channel to consume from.
// Raw outputs receive messages after marshaling to bytes.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawOutput(name string, matcher Matcher) (<-chan *RawMessage, error) {
	e.cfg.Logger.Info("Adding raw output", "output", name)
	out, err := e.distributor.AddOutput(func(msg *Message) bool {
		return matcher == nil || matcher.Match(msg.Attributes)
	})
	if err != nil {
		return nil, err
	}
	return e.marshal(out), nil
}

// Start begins processing messages. Returns a done channel that closes
// when the engine has fully stopped.
//
// Shutdown behavior:
//
// For graceful shutdown, close all input channels first, then cancel the context.
// This allows the pipeline to drain naturally without message loss.
//
// If context is cancelled with inputs still open:
//   - ShutdownTimeout <= 0: waits indefinitely for inputs to close
//   - ShutdownTimeout > 0: merger forces shutdown after timeout, may drop
//     messages still in input channels
//
// Messages that pass the merger are guaranteed to be delivered to outputs.
// The router, distributor, and marshal pipes wait for their inputs to close,
// creating a cascading drain that ensures delivery.
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	e.started = true
	e.mu.Unlock()

	e.cfg.Logger.Info("Starting engine",
		"buffer_size", e.cfg.BufferSize)

	// Propagate context cancellation to shutdown channel
	// This triggers shutdown of marshal/unmarshal pipes
	go func() {
		<-ctx.Done()
		close(e.shutdown)
	}()

	// 1. Start merger
	merged, err := e.merger.Merge(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Route messages to handlers
	handled, err := e.router.Pipe(ctx, merged)
	if err != nil {
		return nil, err
	}

	// 3. Start distributor and return its done channel directly
	return e.distributor.Distribute(ctx, handled)
}

// applyTypedInputMatcher filters typed messages using the matcher.
func (e *Engine) applyTypedInputMatcher(name string, in <-chan *Message, matcher Matcher) <-chan *Message {
	if matcher == nil {
		return in
	}

	return channel.Filter(in, func(msg *Message) bool {
		if matcher.Match(msg.Attributes) {
			return true
		}
		err := ErrInputRejected
		msg.Nack(err)
		e.cfg.Logger.Warn("Input message rejected by matcher",
			"input", name,
			"attributes", msg.Attributes)
		e.cfg.ErrorHandler(msg, ErrInputRejected)
		return false
	})
}

// applyRawInputMatcher filters raw messages using the matcher.
func (e *Engine) applyRawInputMatcher(name string, in <-chan *RawMessage, matcher Matcher) <-chan *RawMessage {
	if matcher == nil {
		return in
	}

	return channel.Filter(in, func(msg *RawMessage) bool {
		if matcher.Match(msg.Attributes) {
			return true
		}
		err := ErrInputRejected
		msg.Nack(err)
		e.cfg.Logger.Warn("Raw input message rejected by matcher",
			"input", name,
			"attributes", msg.Attributes)
		e.handleRawError(msg, ErrInputRejected)
		return false
	})
}

// handleRawError calls ErrorHandler with a Message wrapper for raw attributes.
func (e *Engine) handleRawError(raw *RawMessage, err error) {
	e.cfg.ErrorHandler(&Message{
		Attributes: raw.Attributes,
		Data:       raw.Data,
		acking:     raw.acking,
	}, err)
}

// unmarshal processes raw messages into typed messages.
// No ShutdownTimeout - waits for input to close naturally, ensuring all
// received messages are forwarded to the merger before exiting.
// Users should close input channels for graceful shutdown.
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message {
	p := NewUnmarshalPipe(e.router, e.cfg.Marshaler, pipe.Config{
		// No ShutdownTimeout: relies on input closing for natural completion.
		// This prevents race where unmarshal and merger timeout simultaneously,
		// potentially dropping messages buffered in unmarshal's output.
		ErrorHandler: func(in any, err error) {
			e.cfg.Logger.Error("Unmarshaling raw message failed", "error", err)
			raw := in.(*RawMessage)
			raw.Nack(err)
			e.handleRawError(raw, err)
		},
	})
	out, _ := p.Pipe(chanContext(e.shutdown), in)
	return out
}

// marshal processes typed messages into raw messages.
// No ShutdownTimeout - waits for distributor to close its output naturally.
// This ensures all messages that reach the distributor are marshaled and delivered.
func (e *Engine) marshal(in <-chan *Message) <-chan *RawMessage {
	p := NewMarshalPipe(e.cfg.Marshaler, pipe.Config{
		// No ShutdownTimeout: relies on distributor closing its output.
		// Distributor has no timeout and waits for router to complete,
		// which waits for merger. This creates cascading drain.
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			e.cfg.Logger.Error("Marshaling message failed", "error", err)
			e.cfg.ErrorHandler(msg, err)
		},
	})
	out, _ := p.Pipe(chanContext(e.shutdown), in)
	return out
}

// chanContext creates a context that is cancelled when the channel is closed.
func chanContext(done <-chan struct{}) context.Context {
	return &channelContext{done: done}
}

type channelContext struct {
	done <-chan struct{}
}

func (c *channelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *channelContext) Done() <-chan struct{}       { return c.done }
func (c *channelContext) Value(key any) any           { return nil }

func (c *channelContext) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}
