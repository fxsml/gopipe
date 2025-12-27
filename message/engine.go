package message

import (
	"context"
	"log/slog"
	"sync"

	"github.com/fxsml/gopipe/channel"
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

	mu      sync.Mutex
	started bool
	done    chan struct{}
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
func (e *Engine) AddInput(ch <-chan *RawMessage, cfg InputConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inputs = append(e.inputs, inputEntry{ch: ch, config: cfg})
	return nil
}

// AddOutput registers an output and returns the channel to consume from.
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *RawMessage {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan *RawMessage, 100)
	e.outputs = append(e.outputs, outputEntry{ch: ch, config: cfg})
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
	e.mu.Unlock()

	// Build the processing pipeline using channel helpers
	filteredInputs := make([]<-chan *RawMessage, 0, len(e.inputs))
	for _, input := range e.inputs {
		// Use channel.Cancel for context-aware input handling
		cancelled := channel.Cancel(ctx, input.ch, func(msg *RawMessage, err error) {
			// Messages in-flight during cancellation are dropped
		})

		// Use channel.Process for filtering with error callback
		// Process returns 0 or 1 messages per input (filter semantics)
		filtered := e.applyInputMatcher(cancelled, input.config.Matcher)
		filteredInputs = append(filteredInputs, filtered)
	}

	// Merge all filtered inputs using channel.Merge
	merged := channel.Merge(filteredInputs...)

	// Process messages using channel.Sink
	sinkDone := channel.Sink(merged, func(raw *RawMessage) {
		e.processMessage(ctx, raw)
	})

	// Wait for sink to complete and close outputs
	go func() {
		<-sinkDone
		for _, out := range e.outputs {
			close(out.ch)
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

func (e *Engine) processMessage(ctx context.Context, raw *RawMessage) {
	// Get CE type from attributes
	ceType, _ := raw.Attributes["type"].(string)

	// Find handler
	handler, ok := e.handlers[ceType]
	if !ok {
		e.errorHandler(&Message{Attributes: raw.Attributes}, ErrNoHandler)
		return
	}

	// Create instance and unmarshal
	instance := handler.NewInput()
	if err := e.marshaler.Unmarshal(raw.Data, instance); err != nil {
		e.errorHandler(&Message{Attributes: raw.Attributes}, err)
		return
	}

	// Create Message with typed data
	msg := &Message{
		Data:       instance,
		Attributes: raw.Attributes,
		a:          raw.a,
	}

	// Handle message
	outputs, err := handler.Handle(ctx, msg)
	if err != nil {
		e.errorHandler(msg, err)
		return
	}

	// Route outputs
	for _, out := range outputs {
		e.routeOutput(ctx, out)
	}

	// Ack original message
	raw.Ack()
}

func (e *Engine) routeOutput(ctx context.Context, msg *Message) {
	// Check loopbacks first
	for _, lb := range e.loopbacks {
		if lb.config.Matcher != nil && lb.config.Matcher.Match(msg) {
			// Loopback - reprocess without marshal/unmarshal
			go func(m *Message) {
				ceType, _ := m.Attributes["type"].(string)
				handler, ok := e.handlers[ceType]
				if !ok {
					e.errorHandler(m, ErrNoHandler)
					return
				}
				outputs, err := handler.Handle(ctx, m)
				if err != nil {
					e.errorHandler(m, err)
					return
				}
				for _, out := range outputs {
					e.routeOutput(ctx, out)
				}
			}(msg)
			return
		}
	}

	// Marshal for output
	data, err := e.marshaler.Marshal(msg.Data)
	if err != nil {
		e.errorHandler(msg, err)
		return
	}

	// Set DataContentType
	if msg.Attributes == nil {
		msg.Attributes = make(Attributes)
	}
	msg.Attributes["datacontenttype"] = e.marshaler.DataContentType()

	raw := &RawMessage{
		Data:       data,
		Attributes: msg.Attributes,
	}

	// Find matching output
	for _, out := range e.outputs {
		if out.config.Matcher == nil || out.config.Matcher.Match(msg) {
			select {
			case out.ch <- raw:
			case <-ctx.Done():
			}
			return
		}
	}

	e.errorHandler(msg, ErrNoMatchingOutput)
}
