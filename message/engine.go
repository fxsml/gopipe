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

// EngineConfig configures the message engine.
type EngineConfig struct {
	Marshaler    Marshaler
	ErrorHandler ErrorHandler
	BufferSize   int // Channel buffer size (default: 100)
}

// Engine orchestrates message flow between inputs, handlers, and outputs.
// Uses a single merger for all inputs (typed and unmarshaled raw) and
// a single distributor for all outputs (typed and pre-marshal raw).
type Engine struct {
	marshaler    Marshaler
	errorHandler ErrorHandler
	bufferSize   int
	router       *Router

	mu          sync.Mutex
	started     bool
	merger      *pipe.Merger[*Message]
	distributor *pipe.Distributor[*Message]
}

// NewEngine creates a new message engine.
func NewEngine(cfg EngineConfig) *Engine {
	eh := cfg.ErrorHandler
	if eh == nil {
		eh = func(msg *Message, err error) {
			slog.Error("engine error", "error", err)
		}
	}

	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = 100
	}

	e := &Engine{
		marshaler:    cfg.Marshaler,
		errorHandler: eh,
		bufferSize:   bufferSize,
		router:       NewRouter(RouterConfig{ErrorHandler: eh, BufferSize: bufferSize}),
	}

	// Create merger upfront - AddInput works before Merge()
	// ShutdownTimeout ensures merger closes output on context cancel
	e.merger = pipe.NewMerger[*Message](pipe.MergerConfig{
		Buffer:          bufferSize,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	// Create distributor upfront - AddOutput works before Distribute()
	e.distributor = pipe.NewDistributor[*Message](pipe.DistributorConfig[*Message]{
		Buffer: bufferSize,
		NoMatchHandler: func(msg *Message) {
			e.errorHandler(msg, ErrNoMatchingOutput)
		},
	})

	return e
}

// AddHandler registers a handler for its CE type.
// The optional Matcher in HandlerConfig is applied after type matching.
func (e *Engine) AddHandler(h Handler, cfg HandlerConfig) error {
	return e.router.AddHandler(h, cfg)
}

// AddInput registers a typed input channel.
// Typed inputs go directly to the merger, bypassing unmarshaling.
// Use for internal messaging, testing, or when data is already typed.
// Can be called before or after Start().
func (e *Engine) AddInput(ch <-chan *Message, cfg InputConfig) error {
	filtered := e.applyTypedInputMatcher(ch, cfg.Matcher)
	_, err := e.merger.AddInput(filtered)
	return err
}

// AddRawInput registers a raw input channel.
// Raw inputs are filtered, unmarshaled, and fed to the merger.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawInput(ch <-chan *RawMessage, cfg RawInputConfig) error {
	filtered := e.applyRawInputMatcher(ch, cfg.Matcher)
	return e.AddInput(e.unmarshal(filtered), InputConfig{})
}

// AddOutput registers a typed output and returns the channel to consume from.
// Typed outputs receive messages directly from the distributor without marshaling.
// Use for internal messaging or when you need typed access to messages.
// Can be called before or after Start().
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message {
	outMatcher := cfg.Matcher
	ch, _ := e.distributor.AddOutput(func(msg *Message) bool {
		return outMatcher == nil || outMatcher.Match(msg.Attributes)
	})
	return ch
}

// AddRawOutput registers a raw output and returns the channel to consume from.
// Raw outputs receive messages after marshaling to bytes.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawOutput(cfg RawOutputConfig) <-chan *RawMessage {
	return e.marshal(e.AddOutput(OutputConfig{Matcher: cfg.Matcher}))
}

// AddLoopback registers internal message re-processing.
// Loopback messages are fed back to the merger without marshal/unmarshal.
// Ordering follows call order: call AddLoopback before AddOutput if loopback
// should have priority when matchers overlap.
func (e *Engine) AddLoopback(cfg LoopbackConfig) error {
	ch := e.AddOutput(OutputConfig{Matcher: cfg.Matcher})
	return e.AddInput(ch, InputConfig{})
}

// Start begins processing messages. Returns a done channel.
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	e.started = true
	e.mu.Unlock()

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
func (e *Engine) applyTypedInputMatcher(in <-chan *Message, matcher Matcher) <-chan *Message {
	if matcher == nil {
		return in
	}

	return channel.Filter(in, func(msg *Message) bool {
		if matcher.Match(msg.Attributes) {
			return true
		}
		e.errorHandler(msg, ErrInputRejected)
		return false
	})
}

// applyRawInputMatcher filters raw messages using the matcher.
func (e *Engine) applyRawInputMatcher(in <-chan *RawMessage, matcher Matcher) <-chan *RawMessage {
	if matcher == nil {
		return in
	}

	return channel.Filter(in, func(msg *RawMessage) bool {
		if matcher.Match(msg.Attributes) {
			return true
		}
		e.handleRawError(msg.Attributes, ErrInputRejected)
		return false
	})
}

// handleRawError calls errorHandler with a Message wrapper for raw attributes.
func (e *Engine) handleRawError(attrs Attributes, err error) {
	e.errorHandler(&Message{Attributes: attrs}, err)
}

// unmarshal processes raw messages into typed messages.
func (e *Engine) unmarshal(in <-chan *RawMessage) <-chan *Message {
	return channel.Process(in, func(raw *RawMessage) []*Message {
		ceType, _ := raw.Attributes["type"].(string)

		entry, ok := e.router.handler(ceType)
		if !ok {
			e.handleRawError(raw.Attributes, ErrNoHandler)
			return nil
		}

		instance := entry.handler.NewInput()
		if err := e.marshaler.Unmarshal(raw.Data, instance); err != nil {
			e.handleRawError(raw.Attributes, err)
			return nil
		}

		return []*Message{{
			Data:       instance,
			Attributes: raw.Attributes,
			a:          raw.a,
		}}
	})
}

// marshal processes typed messages into raw messages.
func (e *Engine) marshal(in <-chan *Message) <-chan *RawMessage {
	return channel.Process(in, func(msg *Message) []*RawMessage {
		data, err := e.marshaler.Marshal(msg.Data)
		if err != nil {
			e.errorHandler(msg, err)
			return nil
		}

		if msg.Attributes == nil {
			msg.Attributes = make(Attributes)
		}
		msg.Attributes["datacontenttype"] = e.marshaler.DataContentType()

		return []*RawMessage{{
			Data:       data,
			Attributes: msg.Attributes,
		}}
	})
}
