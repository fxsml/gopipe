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
}

// Engine orchestrates message flow between inputs, handlers, and outputs.
// Uses two mergers: RawMerger for raw inputs (broker integration) and
// TypedMerger for typed inputs, loopback, and internal routing.
// Uses two distributors: typed Distributor for loopbacks and typed outputs,
// and rawDistributor for raw outputs (after single marshal pipe).
type Engine struct {
	marshaler    Marshaler
	errorHandler ErrorHandler
	router       *Router

	// Typed outputs (directly from Distributor, no marshal)
	typedOutputs []typedOutputEntry
	// Raw outputs (from Distributor → Marshal → RawDistributor)
	rawOutputs []rawOutputEntry
	// Loopbacks (from Distributor back to TypedMerger)
	loopbacks []loopbackEntry

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

type loopbackEntry struct {
	config LoopbackConfig
}

// NewEngine creates a new message engine.
func NewEngine(cfg EngineConfig) *Engine {
	eh := cfg.ErrorHandler
	if eh == nil {
		eh = func(msg *Message, err error) {
			slog.Error("engine error", "error", err)
		}
	}

	e := &Engine{
		marshaler:    cfg.Marshaler,
		errorHandler: eh,
		router:       NewRouter(RouterConfig{ErrorHandler: eh}),
		done:         make(chan struct{}),
	}

	// Create mergers upfront - AddInput works before Merge()
	e.typedMerger = pipe.NewMerger[*Message](pipe.MergerConfig{Buffer: 100})
	e.rawMerger = pipe.NewMerger[*RawMessage](pipe.MergerConfig{Buffer: 100})

	// Create distributor upfront - AddOutput works before Distribute()
	e.distributor = pipe.NewDistributor[*Message](pipe.DistributorConfig[*Message]{
		Buffer: 100,
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
		ch := make(chan *Message, 100)
		e.typedOutputs = append(e.typedOutputs, typedOutputEntry{ch: ch, config: cfg})
		return ch
	}

	// After Start: add directly to distributor, return its channel (no forwarding)
	outMatcher := cfg.Matcher
	ch, _ := e.distributor.AddOutput(func(msg *Message) bool {
		return outMatcher == nil || outMatcher.Match(msg)
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
		ch := make(chan *RawMessage, 100)
		e.rawOutputs = append(e.rawOutputs, rawOutputEntry{ch: ch, config: cfg})
		return ch
	}

	// After Start: add directly to rawDistributor if it exists
	outMatcher := cfg.Matcher
	if e.rawDistributor != nil {
		ch, _ := e.rawDistributor.AddOutput(func(msg *RawMessage) bool {
			m := &Message{Attributes: msg.Attributes}
			return outMatcher == nil || outMatcher.Match(m)
		})
		return ch
	}

	// No raw distributor (no raw outputs at start) - fall back to per-output marshal
	msgCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
		return outMatcher == nil || outMatcher.Match(msg)
	})

	// Start marshal pipe for this output
	marshalPipe := e.createMarshalPipe()
	rawCh, _ := marshalPipe.Pipe(e.ctx, msgCh)
	return rawCh
}

// AddLoopback registers internal message re-processing.
// Loopback messages are fed back to the TypedMerger without marshal/unmarshal.
func (e *Engine) AddLoopback(cfg LoopbackConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loopbacks = append(e.loopbacks, loopbackEntry{config: cfg})
	return nil
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

	// 1. Create loopback channel for feedback
	var loopbackIn chan *Message
	if len(e.loopbacks) > 0 {
		loopbackIn = make(chan *Message, 100)
		e.typedMerger.AddInput(loopbackIn)
	}

	// 2. Set up raw input path if any raw inputs were added
	if e.hasRawInputs {
		// Merge raw inputs
		rawMerged, err := e.rawMerger.Merge(ctx)
		if err != nil {
			return nil, err
		}

		// Unmarshal raw messages and feed to typed merger
		unmarshalPipe := e.createUnmarshalPipe()
		unmarshaled, err := unmarshalPipe.Pipe(ctx, rawMerged)
		if err != nil {
			return nil, err
		}

		e.typedMerger.AddInput(unmarshaled)
	}

	// 3. Start typed merger
	typedMerged, err := e.typedMerger.Merge(ctx)
	if err != nil {
		return nil, err
	}

	// 4. Route messages to handlers
	handled, err := e.router.Pipe(ctx, typedMerged)
	if err != nil {
		return nil, err
	}

	// 5. Add loopback outputs first (first-match-wins)
	for _, lb := range e.loopbacks {
		lbMatcher := lb.config.Matcher
		loopbackCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return lbMatcher != nil && lbMatcher.Match(msg)
		})

		// Feed loopback messages back to typed merger
		go func(ch <-chan *Message) {
			for msg := range ch {
				select {
				case loopbackIn <- msg:
				case <-ctx.Done():
					return
				}
			}
		}(loopbackCh)
	}

	// 6. Add typed outputs (forwarding needed due to ordering constraints)
	for _, output := range e.typedOutputs {
		outMatcher := output.config.Matcher
		msgCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return outMatcher == nil || outMatcher.Match(msg)
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

	// 7. Set up raw output infrastructure (single marshal pipe → raw distributor)
	if len(e.rawOutputs) > 0 {
		// Add catch-all for raw-bound messages (after loopbacks and typed outputs)
		rawBound, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return true // catch all remaining
		})

		// Create ONE marshal pipe (shared by all raw outputs)
		marshalPipe := e.createMarshalPipe()
		marshaled, _ := marshalPipe.Pipe(ctx, rawBound)

		// Create raw distributor
		e.rawDistributor = pipe.NewDistributor[*RawMessage](pipe.DistributorConfig[*RawMessage]{
			Buffer: 100,
			NoMatchHandler: func(msg *RawMessage) {
				e.errorHandler(&Message{Attributes: msg.Attributes}, ErrNoMatchingOutput)
			},
		})

		// Add each raw output to raw distributor
		for _, output := range e.rawOutputs {
			outMatcher := output.config.Matcher
			rawCh, _ := e.rawDistributor.AddOutput(func(msg *RawMessage) bool {
				m := &Message{Attributes: msg.Attributes}
				return outMatcher == nil || outMatcher.Match(m)
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

	// 8. Start distributor
	distributeDone, err := e.distributor.Distribute(ctx, handled)
	if err != nil {
		return nil, err
	}

	// 9. Wait for completion (either distributor finishes or context cancelled)
	go func() {
		select {
		case <-distributeDone:
		case <-ctx.Done():
		}
		if loopbackIn != nil {
			close(loopbackIn)
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

	return channel.Process(in, func(msg *Message) []*Message {
		if matcher.Match(msg) {
			return []*Message{msg}
		}
		e.errorHandler(msg, ErrInputRejected)
		return nil
	})
}

// applyRawInputMatcher filters raw messages using the matcher.
func (e *Engine) applyRawInputMatcher(in <-chan *RawMessage, matcher Matcher) <-chan *RawMessage {
	if matcher == nil {
		return in
	}

	return channel.Process(in, func(msg *RawMessage) []*RawMessage {
		m := &Message{Attributes: msg.Attributes}
		if matcher.Match(m) {
			return []*RawMessage{msg}
		}
		e.errorHandler(m, ErrInputRejected)
		return nil
	})
}

// createUnmarshalPipe creates a pipe that unmarshals RawMessage to typed Message.
func (e *Engine) createUnmarshalPipe() *pipe.ProcessPipe[*RawMessage, *Message] {
	return pipe.NewProcessPipe(
		func(ctx context.Context, raw *RawMessage) ([]*Message, error) {
			ceType, _ := raw.Attributes["type"].(string)

			entry, ok := e.router.handler(ceType)
			if !ok {
				return nil, ErrNoHandler
			}

			instance := entry.handler.NewInput()
			if err := e.marshaler.Unmarshal(raw.Data, instance); err != nil {
				return nil, err
			}

			return []*Message{{
				Data:       instance,
				Attributes: raw.Attributes,
				a:          raw.a,
			}}, nil
		},
		pipe.Config{
			ErrorHandler: func(in any, err error) {
				raw := in.(*RawMessage)
				e.errorHandler(&Message{Attributes: raw.Attributes}, err)
			},
		},
	)
}

// createMarshalPipe creates a pipe that marshals Message to RawMessage.
func (e *Engine) createMarshalPipe() *pipe.ProcessPipe[*Message, *RawMessage] {
	return pipe.NewProcessPipe(
		func(ctx context.Context, msg *Message) ([]*RawMessage, error) {
			data, err := e.marshaler.Marshal(msg.Data)
			if err != nil {
				return nil, err
			}

			if msg.Attributes == nil {
				msg.Attributes = make(Attributes)
			}
			msg.Attributes["datacontenttype"] = e.marshaler.DataContentType()

			return []*RawMessage{{
				Data:       data,
				Attributes: msg.Attributes,
			}}, nil
		},
		pipe.Config{
			ErrorHandler: func(in any, err error) {
				msg := in.(*Message)
				e.errorHandler(msg, err)
			},
		},
	)
}
