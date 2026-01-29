package message

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
)

// ErrorHandler processes engine errors.
type ErrorHandler func(msg *Message, err error)

// Plugin registers handlers, middleware, or other components with an engine.
type Plugin func(*Engine) error

// EngineConfig configures the message engine.
type EngineConfig struct {
	// Marshaler converts between typed data and raw bytes.
	Marshaler Marshaler
	// BufferSize is the engine buffer size for merger and distributor (default: 100).
	BufferSize int
	// RouterPool configures the router's worker pool (default: 1 worker, 100 buffer).
	RouterPool PoolConfig
	// ErrorHandler is called on processing errors (default: no-op).
	// Errors are logged via Logger; use ErrorHandler for custom handling
	// like metrics, alerting, or recovery logic.
	ErrorHandler ErrorHandler
	// Logger for engine events (default: slog.Default()).
	Logger Logger
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	// On forced shutdown, remaining messages are drained and reported via ErrorHandler.
	ShutdownTimeout time.Duration
}

func (c EngineConfig) parse() EngineConfig {
	if c.Marshaler == nil {
		c.Marshaler = NewJSONMarshaler()
	}
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(msg *Message, err error) {}
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	c.RouterPool = c.RouterPool.parse()
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
	merger      *Merger
	distributor *Distributor
	shutdown    chan struct{} // Signals marshal/unmarshal pipes to stop
}

// NewEngine creates a new message engine.
func NewEngine(cfg EngineConfig) *Engine {
	cfg = cfg.parse()

	e := &Engine{
		cfg:      cfg,
		shutdown: make(chan struct{}),
		router: NewRouter(PipeConfig{
			Pool:            cfg.RouterPool,
			ShutdownTimeout: cfg.ShutdownTimeout,
			Logger:          cfg.Logger,
			ErrorHandler:    cfg.ErrorHandler,
		}),
	}

	// Create merger upfront - AddInput works before Merge()
	// ShutdownTimeout ensures merger closes output on context cancel
	e.merger = NewMerger(MergerConfig{
		BufferSize:      cfg.BufferSize,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          cfg.Logger,
		ErrorHandler:    cfg.ErrorHandler,
	})

	// Create distributor upfront - AddOutput works before Distribute()
	e.distributor = NewDistributor(DistributorConfig{
		BufferSize:      cfg.BufferSize,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          cfg.Logger,
		ErrorHandler:    cfg.ErrorHandler,
	})

	return e
}

// AddHandler registers a handler to the default pool.
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
func (e *Engine) AddPlugin(plugins ...Plugin) error {
	for _, p := range plugins {
		e.cfg.Logger.Info("Adding plugin", "component", "engine", "plugin", funcName(p))
		if err := p(e); err != nil {
			return err
		}
	}
	return nil
}

// AddInput registers a typed input channel.
// Typed inputs go directly to the merger, bypassing unmarshaling.
// Use for internal messaging, testing, or when data is already typed.
// Can be called before or after Start().
func (e *Engine) AddInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error) {
	e.cfg.Logger.Info("Adding input", "component", "engine", "input", name)
	filtered := e.applyTypedInputMatcher(name, ch, matcher)
	return e.merger.AddInput(filtered)
}

// AddRawInput registers a raw input channel.
// Raw inputs are filtered, unmarshaled, and fed to the merger.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawInput(name string, matcher Matcher, ch <-chan *RawMessage) (<-chan struct{}, error) {
	e.cfg.Logger.Info("Adding raw input", "component", "engine", "input", name)
	filtered := e.applyRawInputMatcher(name, ch, matcher)
	return e.merger.AddInput(e.unmarshal(filtered))
}

// AddOutput registers a typed output and returns the channel to consume from.
// Typed outputs receive messages directly from the distributor without marshaling.
// Use for internal messaging or when you need typed access to messages.
// Can be called before or after Start().
func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error) {
	e.cfg.Logger.Info("Adding output", "component", "engine", "output", name)
	return e.distributor.AddOutput(matcher)
}

// AddRawOutput registers a raw output and returns the channel to consume from.
// Raw outputs receive messages after marshaling to bytes.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawOutput(name string, matcher Matcher) (<-chan *RawMessage, error) {
	e.cfg.Logger.Info("Adding raw output", "component", "engine", "output", name)
	out, err := e.distributor.AddOutput(matcher)
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
//   - ShutdownTimeout <= 0: forces immediate shutdown, drops messages in inputs
//   - ShutdownTimeout > 0: waits up to timeout for natural completion, then
//     forces shutdown and drops messages still in input channels
//
// Messages that pass the merger are guaranteed to be delivered to outputs.
// The router, distributor, and marshal pipes use the same ShutdownTimeout,
// creating a cascading drain that ensures delivery after the merger.
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	e.started = true
	e.mu.Unlock()

	e.cfg.Logger.Info("Starting",
		"component", "engine",
		"buffer_size", e.cfg.BufferSize,
		"router_workers", e.cfg.RouterPool.Workers)

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

	// 3. Start distributor
	distDone, err := e.distributor.Distribute(ctx, handled)
	if err != nil {
		return nil, err
	}

	// 4. Create done channel that closes when distributor finishes.
	// Shutdown: close(e.shutdown) signals marshal/unmarshal pipes immediately.
	// Each component uses ShutdownTimeout independently; cascade works via
	// channel closures (unmarshal→merger→router→distributor).
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		e.cfg.Logger.Info("Shutdown initiated", "component", "engine")
		close(e.shutdown)
		<-distDone
		e.cfg.Logger.Info("Stopped", "component", "engine")
		close(done)
	}()

	return done, nil
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
			"component", "engine",
			"error", err,
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
			"component", "engine",
			"error", err,
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
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message {
	p := NewUnmarshalPipe(e.router, e.cfg.Marshaler, PipeConfig{
		ShutdownTimeout: e.cfg.ShutdownTimeout,
		Logger:          e.cfg.Logger,
		ErrorHandler:    e.cfg.ErrorHandler,
	})
	out, _ := p.Pipe(chanContext(e.shutdown), in)
	return out
}

// marshal processes typed messages into raw messages.
func (e *Engine) marshal(in <-chan *Message) <-chan *RawMessage {
	p := NewMarshalPipe(e.cfg.Marshaler, PipeConfig{
		ShutdownTimeout: e.cfg.ShutdownTimeout,
		Logger:          e.cfg.Logger,
		ErrorHandler:    e.cfg.ErrorHandler,
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
