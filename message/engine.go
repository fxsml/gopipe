package message

import (
	"context"
	"log/slog"
	"sync"

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
// Uses two mergers: RawMerger for raw inputs (broker integration) and
// TypedMerger for typed inputs, loopback, and internal routing.
// Uses two distributors: typed Distributor for loopbacks and typed outputs,
// and rawDistributor for raw outputs (after single marshal pipe).
type Engine struct {
	marshaler    Marshaler
	errorHandler ErrorHandler
	bufferSize   int
	router       *Router

	// Typed outputs (directly from Distributor, no marshal)
	typedOutputs []typedOutputEntry
	// Raw outputs (from Distributor → Marshal → RawDistributor)
	rawOutputs []rawOutputEntry

	mu             sync.Mutex
	started        bool
	hasRawInputs   bool // Track if any raw inputs were added
	done           chan struct{}
	ctx            context.Context
	rawMerger      *pipe.Merger[*RawMessage]
	typedMerger    *pipe.Merger[*Message]
	distributor    *pipe.Distributor[*Message]
	rawDistributor *pipe.Distributor[*RawMessage]
}

type typedOutputEntry struct {
	ch     chan *Message
	config OutputConfig
}

type rawOutputEntry struct {
	ch     chan *RawMessage
	config RawOutputConfig
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
		done:         make(chan struct{}),
	}

	// Create mergers upfront - AddInput works before Merge()
	e.typedMerger = pipe.NewMerger[*Message](pipe.MergerConfig{Buffer: bufferSize})
	e.rawMerger = pipe.NewMerger[*RawMessage](pipe.MergerConfig{Buffer: bufferSize})

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
// Typed inputs go directly to the TypedMerger, bypassing unmarshaling.
// Use for internal messaging, testing, or when data is already typed.
// Can be called before or after Start().
func (e *Engine) AddInput(ch <-chan *Message, cfg InputConfig) error {
	filtered := e.applyTypedInputMatcher(ch, cfg.Matcher)
	_, err := e.typedMerger.AddInput(filtered)
	return err
}

// AddRawInput registers a raw input channel.
// Raw inputs go through RawMerger → Unmarshal → TypedMerger.
// Use for broker integration (Kafka, NATS, RabbitMQ, etc.).
// Can be called before or after Start().
func (e *Engine) AddRawInput(ch <-chan *RawMessage, cfg RawInputConfig) error {
	e.mu.Lock()
	e.hasRawInputs = true
	e.mu.Unlock()

	filtered := e.applyRawInputMatcher(ch, cfg.Matcher)
	_, err := e.rawMerger.AddInput(filtered)
	return err
}

// AddOutput registers a typed output and returns the channel to consume from.
// Typed outputs receive messages directly from the Distributor without marshaling.
// Use for internal messaging or when you need typed access to messages.
// Can be called before or after Start().
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started {
		// Before Start: store config and intermediate channel (ordering matters)
		ch := make(chan *Message, e.bufferSize)
		e.typedOutputs = append(e.typedOutputs, typedOutputEntry{ch: ch, config: cfg})
		return ch
	}

	// After Start: add directly to distributor, return its channel (no forwarding)
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
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started {
		// Before Start: store config and intermediate channel (ordering matters)
		ch := make(chan *RawMessage, e.bufferSize)
		e.rawOutputs = append(e.rawOutputs, rawOutputEntry{ch: ch, config: cfg})
		return ch
	}

	// After Start: add directly to rawDistributor if it exists
	outMatcher := cfg.Matcher
	if e.rawDistributor != nil {
		ch, _ := e.rawDistributor.AddOutput(func(msg *RawMessage) bool {
			return outMatcher == nil || outMatcher.Match(msg.Attributes)
		})
		return ch
	}

	// No raw distributor (no raw outputs at start) - fall back to per-output marshal
	msgCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
		return outMatcher == nil || outMatcher.Match(msg.Attributes)
	})

	// Marshal messages for this output
	return e.marshal(msgCh)
}

// AddLoopback registers internal message re-processing.
// Loopback messages are fed back to the TypedMerger without marshal/unmarshal.
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
	e.ctx = ctx
	e.mu.Unlock()

	// 1. Set up raw input path if any raw inputs were added
	if e.hasRawInputs {
		// Merge raw inputs
		rawMerged, err := e.rawMerger.Merge(ctx)
		if err != nil {
			return nil, err
		}

		// Unmarshal raw messages and feed to typed merger
		e.typedMerger.AddInput(e.unmarshal(rawMerged))
	}

	// 2. Start typed merger
	typedMerged, err := e.typedMerger.Merge(ctx)
	if err != nil {
		return nil, err
	}

	// 3. Route messages to handlers
	handled, err := e.router.Pipe(ctx, typedMerged)
	if err != nil {
		return nil, err
	}

	// 4. Add typed outputs (forwarding needed due to pre-Start channel returns)
	for _, output := range e.typedOutputs {
		outMatcher := output.config.Matcher
		msgCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return outMatcher == nil || outMatcher.Match(msg.Attributes)
		})

		// Forward to output channel
		go func(out chan *Message, msgCh <-chan *Message) {
			for msg := range msgCh {
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
			close(out)
		}(output.ch, msgCh)
	}

	// 5. Set up raw output infrastructure (single marshal → raw distributor)
	if len(e.rawOutputs) > 0 {
		// Add catch-all for raw-bound messages (after typed outputs)
		rawBound, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return true // catch all remaining
		})

		// Marshal messages (shared by all raw outputs)
		marshaled := e.marshal(rawBound)

		// Create raw distributor
		e.rawDistributor = pipe.NewDistributor[*RawMessage](pipe.DistributorConfig[*RawMessage]{
			Buffer: e.bufferSize,
			NoMatchHandler: func(msg *RawMessage) {
				e.handleRawError(msg.Attributes, ErrNoMatchingOutput)
			},
		})

		// Add each raw output to raw distributor
		for _, output := range e.rawOutputs {
			outMatcher := output.config.Matcher
			rawCh, _ := e.rawDistributor.AddOutput(func(msg *RawMessage) bool {
				return outMatcher == nil || outMatcher.Match(msg.Attributes)
			})

			// Forward to output channel
			go func(out chan *RawMessage, rawCh <-chan *RawMessage) {
				for raw := range rawCh {
					select {
					case out <- raw:
					case <-ctx.Done():
						return
					}
				}
				close(out)
			}(output.ch, rawCh)
		}

		// Start raw distributor
		e.rawDistributor.Distribute(ctx, marshaled)
	}

	// 6. Start distributor
	distributeDone, err := e.distributor.Distribute(ctx, handled)
	if err != nil {
		return nil, err
	}

	// 7. Wait for completion (either distributor finishes or context cancelled)
	go func() {
		select {
		case <-distributeDone:
		case <-ctx.Done():
		}
		close(e.done)
	}()

	return e.done, nil
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
