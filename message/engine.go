package message

import (
	"context"
	"log/slog"
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
	// BufferSize is the engine buffer size for merger and distributor (default: 100).
	BufferSize int
	// RouterBufferSize is the router's internal buffer (0 = inherit BufferSize).
	RouterBufferSize int
	// RouterPool configures the default worker pool (Workers default: 1, BufferSize 0 = inherit RouterBufferSize).
	RouterPool PoolConfig
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
	// Inheritance: BufferSize → RouterBufferSize → RouterPool.BufferSize
	if c.RouterBufferSize <= 0 {
		c.RouterBufferSize = c.BufferSize
	}
	if c.RouterPool.BufferSize <= 0 {
		c.RouterPool.BufferSize = c.RouterBufferSize
	}
	if c.RouterPool.Workers <= 0 {
		c.RouterPool.Workers = 1
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

	// Graceful shutdown support
	tracker          *messageTracker
	wrapperWg        sync.WaitGroup
	loopbackShutdown chan struct{} // Closed to break loopback cycles
}

// NewEngine creates a new message engine.
func NewEngine(cfg EngineConfig) *Engine {
	cfg = cfg.parse()

	e := &Engine{
		cfg:              cfg,
		shutdown:         make(chan struct{}),
		loopbackShutdown: make(chan struct{}),
		tracker:          newMessageTracker(),
		router: NewRouter(RouterConfig{
			ErrorHandler: cfg.ErrorHandler,
			BufferSize:   cfg.RouterBufferSize,
			Pool:         cfg.RouterPool,
			Logger:       cfg.Logger,
		}),
	}

	// Add tracking middleware as innermost (applied last, runs first after handler)
	// This tracks when handlers drop or multiply messages
	_ = e.router.Use(e.trackingMiddleware())

	// Create merger upfront - AddInput works before Merge()
	// ShutdownTimeout ensures merger closes output on context cancel
	e.merger = pipe.NewMerger[*Message](pipe.MergerConfig{
		Buffer:          cfg.BufferSize,
		ShutdownTimeout: cfg.ShutdownTimeout,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			e.cfg.Logger.Warn("Message merge failed",
				"component", "engine",
				"error", err,
				"attributes", msg.Attributes)
			e.cfg.ErrorHandler(msg, err)
		},
	})

	// Create distributor upfront - AddOutput works before Distribute()
	// Distributor has no ShutdownTimeout - it waits for input to close naturally,
	// ensuring all messages that pass the merger are delivered to outputs.
	e.distributor = pipe.NewDistributor(pipe.DistributorConfig[*Message]{
		Buffer: cfg.BufferSize,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			e.cfg.Logger.Warn("Message distribution failed",
				"component", "engine",
				"error", err,
				"attributes", msg.Attributes)
			e.cfg.ErrorHandler(msg, err)
		},
	})

	return e
}

// AddHandler registers a handler to the default pool.
// The optional matcher is applied after type matching.
func (e *Engine) AddHandler(name string, matcher Matcher, h Handler) error {
	return e.router.AddHandler(name, matcher, h)
}

// AddPoolWithConfig creates a named worker pool.
// Must be called before Start().
func (e *Engine) AddPoolWithConfig(name string, cfg PoolConfig) error {
	return e.router.AddPoolWithConfig(name, cfg)
}

// AddHandlerToPool registers a handler to a named pool.
// The optional matcher is applied after type matching.
// Must be called before Start().
func (e *Engine) AddHandlerToPool(name string, matcher Matcher, h Handler, pool string) error {
	return e.router.AddHandlerToPool(name, matcher, h, pool)
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
	wrapped := e.wrapInputChannel(filtered)
	return e.merger.AddInput(wrapped)
}

// AddRawInput registers a raw input channel.
// Raw inputs are filtered, unmarshaled, and fed to the merger.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawInput(name string, matcher Matcher, ch <-chan *RawMessage) (<-chan struct{}, error) {
	e.cfg.Logger.Info("Adding raw input", "component", "engine", "input", name)
	filtered := e.applyRawInputMatcher(name, ch, matcher)
	wrapped := e.wrapRawInputChannel(filtered)
	return e.merger.AddInput(e.unmarshal(wrapped))
}

// AddLoopbackInput registers a loopback input channel.
// Loopback inputs are NOT tracked by the message tracker - they are internal cycles.
// Use the plugin.Loopback variants instead of calling this directly.
func (e *Engine) AddLoopbackInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error) {
	e.cfg.Logger.Info("Adding loopback input", "component", "engine", "input", name)
	filtered := e.applyTypedInputMatcher(name, ch, matcher)
	// No wrapping - loopback messages are not tracked
	return e.merger.AddInput(filtered)
}

// AddOutput registers a typed output and returns the channel to consume from.
// Typed outputs receive messages directly from the distributor without marshaling.
// Use for internal messaging or when you need typed access to messages.
// Can be called before or after Start().
func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error) {
	e.cfg.Logger.Info("Adding output", "component", "engine", "output", name)
	out, err := e.distributor.AddOutput(func(msg *Message) bool {
		return matcher == nil || matcher.Match(msg.Attributes)
	})
	if err != nil {
		return nil, err
	}
	return e.wrapOutputChannel(out), nil
}

// AddRawOutput registers a raw output and returns the channel to consume from.
// Raw outputs receive messages after marshaling to bytes.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawOutput(name string, matcher Matcher) (<-chan *RawMessage, error) {
	e.cfg.Logger.Info("Adding raw output", "component", "engine", "output", name)
	out, err := e.distributor.AddOutput(func(msg *Message) bool {
		return matcher == nil || matcher.Match(msg.Attributes)
	})
	if err != nil {
		return nil, err
	}
	wrapped := e.wrapOutputChannel(out)
	return e.marshal(wrapped), nil
}

// AddLoopbackOutput registers a loopback output for graceful shutdown coordination.
// Loopback outputs are NOT tracked - they form internal cycles.
// The Engine wraps these outputs to control when they close during shutdown.
// Use the plugin.Loopback variants instead of calling this directly.
func (e *Engine) AddLoopbackOutput(name string, matcher Matcher) (<-chan *Message, error) {
	e.cfg.Logger.Info("Adding loopback output", "component", "engine", "output", name)

	// Use regular distributor output
	distOut, err := e.distributor.AddOutput(func(msg *Message) bool {
		return matcher == nil || matcher.Match(msg.Attributes)
	})
	if err != nil {
		return nil, err
	}

	// Wrap with Engine-controlled shutdown
	return e.wrapLoopbackOutputChannel(distOut), nil
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

	e.cfg.Logger.Info("Starting",
		"component", "engine",
		"buffer_size", e.cfg.BufferSize,
		"router_workers", e.cfg.RouterPool.Workers)

	// Shutdown orchestration goroutine
	go func() {
		<-ctx.Done()

		e.cfg.Logger.Info("Shutdown initiated", "component", "engine")

		// Signal no more external inputs expected
		e.tracker.close()

		// Wait for pipeline to drain or timeout
		if e.cfg.ShutdownTimeout > 0 {
			select {
			case <-e.tracker.drained():
				e.cfg.Logger.Debug("Pipeline drained, closing loopback outputs", "component", "engine")
			case <-time.After(e.cfg.ShutdownTimeout):
				e.cfg.Logger.Warn("Shutdown timeout reached, forcing loopback close", "component", "engine")
			}
		} else {
			<-e.tracker.drained()
			e.cfg.Logger.Debug("Pipeline drained, closing loopback outputs", "component", "engine")
		}

		// Close loopback outputs to break cycles
		close(e.loopbackShutdown)

		// Close shutdown channel to trigger marshal/unmarshal cleanup
		close(e.shutdown)
	}()

	// 1. Start merger
	merged, err := e.merger.Merge(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Route messages to handlers (with tracking middleware)
	handled, err := e.router.Pipe(ctx, merged)
	if err != nil {
		return nil, err
	}

	// 3. Start distributor
	distDone, err := e.distributor.Distribute(ctx, handled)
	if err != nil {
		return nil, err
	}

	// 4. Create done channel that waits for both distributor and wrapper cleanup
	done := make(chan struct{})
	go func() {
		<-distDone
		e.wrapperWg.Wait()
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
// No ShutdownTimeout - waits for input to close naturally, ensuring all
// received messages are forwarded to the merger before exiting.
// Users should close input channels for graceful shutdown.
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message {
	p := NewUnmarshalPipe(e.router, e.cfg.Marshaler, pipe.Config{
		// No ShutdownTimeout: relies on input closing for natural completion.
		// This prevents race where unmarshal and merger timeout simultaneously,
		// potentially dropping messages buffered in unmarshal's output.
		ErrorHandler: func(in any, err error) {
			raw := in.(*RawMessage)
			e.cfg.Logger.Error("Unmarshaling raw message failed",
				"component", "unmarshaler",
				"error", err,
				"attributes", raw.Attributes)
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
			e.cfg.Logger.Error("Marshaling message failed",
				"component", "marshaler",
				"error", err,
				"attributes", msg.Attributes)
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

// shutdownIfTimeout returns e.shutdown if ShutdownTimeout > 0, else nil.
func (e *Engine) shutdownIfTimeout() <-chan struct{} {
	if e.cfg.ShutdownTimeout > 0 {
		return e.shutdown
	}
	return nil
}

// wrapChannel forwards values from in, calling onReceive for each.
// Stops when in closes or done closes (if non-nil).
func wrapChannel[T any](in <-chan T, done <-chan struct{}, onReceive func(), wg *sync.WaitGroup, bufSize int) <-chan T {
	out := make(chan T, bufSize)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(out)
		for {
			select {
			case val, ok := <-in:
				if !ok {
					return
				}
				if onReceive != nil {
					onReceive()
				}
				select {
				case out <- val:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()
	return out
}

// wrapInputChannel wraps an input channel to track message entry.
func (e *Engine) wrapInputChannel(ch <-chan *Message) <-chan *Message {
	return wrapChannel(ch, e.shutdownIfTimeout(), e.tracker.enter, &e.wrapperWg, 0)
}

// wrapRawInputChannel wraps a raw input channel to track message entry.
func (e *Engine) wrapRawInputChannel(ch <-chan *RawMessage) <-chan *RawMessage {
	return wrapChannel(ch, e.shutdownIfTimeout(), e.tracker.enter, &e.wrapperWg, 0)
}

// wrapOutputChannel wraps an output channel to track message exit.
func (e *Engine) wrapOutputChannel(ch <-chan *Message) <-chan *Message {
	return wrapChannel(ch, e.shutdown, e.tracker.exit, &e.wrapperWg, e.cfg.BufferSize)
}

// wrapLoopbackOutputChannel wraps a loopback output channel.
// Loopback outputs are NOT tracked (no Exit call) since they form internal cycles.
func (e *Engine) wrapLoopbackOutputChannel(ch <-chan *Message) <-chan *Message {
	return wrapChannel(ch, e.loopbackShutdown, nil, &e.wrapperWg, e.cfg.BufferSize)
}

// trackingMiddleware creates middleware that adjusts the message tracker
// when handlers drop messages (return 0) or multiply them (return N > 1).
func (e *Engine) trackingMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			results, err := next(ctx, msg)

			if err != nil {
				// Error = message consumed without producing output
				e.tracker.exit()
				return nil, err
			}

			switch len(results) {
			case 0:
				// Dropped - message consumed without producing output
				e.tracker.exit()
			case 1:
				// 1:1 transform - no change to count
			default:
				// Multiplied - N-1 new messages added
				for i := 1; i < len(results); i++ {
					e.tracker.enter()
				}
			}

			return results, nil
		}
	}
}
