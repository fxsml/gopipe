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
type Engine struct {
	marshaler    Marshaler
	errorHandler ErrorHandler
	handlers     map[string]Handler
	inputs       []inputEntry
	outputs      []outputEntry
	loopbacks    []loopbackEntry

	mu          sync.Mutex
	started     bool
	done        chan struct{}
	ctx         context.Context
	merger      *pipe.Merger[*Message]
	distributor *pipe.Distributor[*Message]
}

type inputEntry struct {
	ch      <-chan *RawMessage
	config  InputConfig
}

type outputEntry struct {
	ch     chan *RawMessage
	config OutputConfig
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
	return &Engine{
		marshaler:    cfg.Marshaler,
		errorHandler: eh,
		handlers:     make(map[string]Handler),
		done:         make(chan struct{}),
	}
}

// AddHandler registers a handler for its CE type.
func (e *Engine) AddHandler(h Handler, cfg HandlerConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[h.EventType()] = h
	return nil
}

// AddInput registers an input channel.
// Can be called before or after Start(). Inputs added after Start are
// immediately connected to the processing pipeline.
func (e *Engine) AddInput(ch <-chan *RawMessage, cfg InputConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started {
		// Store for later processing in Start()
		e.inputs = append(e.inputs, inputEntry{ch: ch, config: cfg})
		return nil
	}

	// Already started - add to merger immediately
	cancelled := channel.Cancel(e.ctx, ch, func(msg *RawMessage, err error) {
		// Messages in-flight during cancellation are dropped
	})
	filtered := e.applyInputMatcher(cancelled, cfg.Matcher)

	unmarshalPipe := e.createUnmarshalPipe()
	unmarshaled, _ := unmarshalPipe.Pipe(e.ctx, filtered)

	_, err := e.merger.AddInput(unmarshaled)
	return err
}

// AddOutput registers an output and returns the channel to consume from.
// Can be called before or after Start(). Outputs added after Start are
// immediately connected to the routing pipeline.
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *RawMessage {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan *RawMessage, 100)

	if !e.started {
		// Store for later processing in Start()
		e.outputs = append(e.outputs, outputEntry{ch: ch, config: cfg})
		return ch
	}

	// Already started - add to distributor immediately
	outMatcher := cfg.Matcher
	msgCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
		return outMatcher == nil || outMatcher.Match(msg)
	})

	// Start marshal pipe for this output
	marshalPipe := e.createMarshalPipe()
	rawCh, _ := marshalPipe.Pipe(e.ctx, msgCh)

	// Forward to output channel
	go func() {
		for raw := range rawCh {
			select {
			case ch <- raw:
			case <-e.ctx.Done():
				return
			}
		}
		close(ch)
	}()

	return ch
}

// AddLoopback registers internal message re-processing.
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

	// 1. Create merger for typed messages
	e.merger = pipe.NewMerger[*Message](pipe.MergerConfig{Buffer: 100})

	// 2. Create loopback channel for feedback (if we have loopbacks)
	var loopbackIn chan *Message
	if len(e.loopbacks) > 0 {
		loopbackIn = make(chan *Message, 100)
		e.merger.AddInput(loopbackIn)
	}

	// 3. For each input: filter → unmarshal → add to merger
	for _, input := range e.inputs {
		cancelled := channel.Cancel(ctx, input.ch, func(msg *RawMessage, err error) {
			// Messages in-flight during cancellation are dropped
		})
		filtered := e.applyInputMatcher(cancelled, input.config.Matcher)

		unmarshalPipe := e.createUnmarshalPipe()
		unmarshaled, _ := unmarshalPipe.Pipe(ctx, filtered)

		e.merger.AddInput(unmarshaled)
	}

	// 4. Start merger
	merged, err := e.merger.Merge(ctx)
	if err != nil {
		return nil, err
	}

	// 5. Start handler pipe
	handlerPipe := e.createHandlerPipe()
	handled, err := handlerPipe.Pipe(ctx, merged)
	if err != nil {
		return nil, err
	}

	// 6. Create distributor for routing
	e.distributor = pipe.NewDistributor[*Message](pipe.DistributorConfig[*Message]{
		Buffer: 100,
		NoMatchHandler: func(msg *Message) {
			e.errorHandler(msg, ErrNoMatchingOutput)
		},
	})

	// 7. Add loopback outputs first (first-match-wins)
	for _, lb := range e.loopbacks {
		lbMatcher := lb.config.Matcher
		loopbackCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return lbMatcher != nil && lbMatcher.Match(msg)
		})

		// Feed loopback messages back to merger (skip unmarshal/marshal)
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

	// 8. For each output: add to distributor with matcher, then marshal
	for _, output := range e.outputs {
		outMatcher := output.config.Matcher
		msgCh, _ := e.distributor.AddOutput(func(msg *Message) bool {
			return outMatcher == nil || outMatcher.Match(msg)
		})

		// Start marshal pipe for this output
		marshalPipe := e.createMarshalPipe()
		rawCh, _ := marshalPipe.Pipe(ctx, msgCh)

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

	// 9. Start distributor
	distributeDone, err := e.distributor.Distribute(ctx, handled)
	if err != nil {
		return nil, err
	}

	// 10. Wait for completion
	go func() {
		<-distributeDone
		if loopbackIn != nil {
			close(loopbackIn)
		}
		close(e.done)
	}()

	return e.done, nil
}

// applyInputMatcher filters messages using the matcher, calling errorHandler for rejected messages.
// Uses channel.Process to map each input to 0 or 1 outputs (filter with side effects).
func (e *Engine) applyInputMatcher(in <-chan *RawMessage, matcher Matcher) <-chan *RawMessage {
	if matcher == nil {
		// No matcher - pass through all messages
		return in
	}

	// Use channel.Process for filter-with-callback semantics
	return channel.Process(in, func(msg *RawMessage) []*RawMessage {
		m := &Message{Attributes: msg.Attributes}
		if matcher.Match(m) {
			return []*RawMessage{msg}
		}
		e.errorHandler(m, ErrInputRejected)
		return nil // Filter out rejected messages
	})
}

// createUnmarshalPipe creates a pipe that unmarshals RawMessage to typed Message.
func (e *Engine) createUnmarshalPipe() *pipe.ProcessPipe[*RawMessage, *Message] {
	return pipe.NewProcessPipe(
		func(ctx context.Context, raw *RawMessage) ([]*Message, error) {
			ceType, _ := raw.Attributes["type"].(string)

			handler, ok := e.handlers[ceType]
			if !ok {
				return nil, ErrNoHandler
			}

			instance := handler.NewInput()
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

// createHandlerPipe creates a pipe that dispatches messages to handlers.
func (e *Engine) createHandlerPipe() *pipe.ProcessPipe[*Message, *Message] {
	return pipe.NewProcessPipe(
		func(ctx context.Context, msg *Message) ([]*Message, error) {
			ceType, _ := msg.Attributes["type"].(string)

			handler, ok := e.handlers[ceType]
			if !ok {
				return nil, ErrNoHandler
			}

			outputs, err := handler.Handle(ctx, msg)
			if err != nil {
				return nil, err
			}

			msg.Ack()
			return outputs, nil
		},
		pipe.Config{
			ErrorHandler: func(in any, err error) {
				msg := in.(*Message)
				e.errorHandler(msg, err)
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
