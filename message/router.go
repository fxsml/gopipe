package message

import (
	"context"
	"log/slog"
	"sync"

	"github.com/fxsml/gopipe/pipe"
)

// handlerEntry holds a handler and its configuration.
type handlerEntry struct {
	handler Handler
	config  HandlerConfig
}

// Router dispatches messages to handlers by CE type.
// Implements Pipe signature for composability with pipe.Apply().
// Uses pipe.ProcessPipe internally for middleware, concurrency, and error handling.
type Router struct {
	mu           sync.RWMutex
	handlers     map[string]handlerEntry
	errorHandler ErrorHandler
	pipeConfig   pipe.Config
}

// RouterConfig configures the message router.
type RouterConfig struct {
	ErrorHandler ErrorHandler
	// PipeConfig allows configuring the underlying ProcessPipe.
	// Buffer, Concurrency, Timeout, etc. can be set here.
	PipeConfig pipe.Config
}

// NewRouter creates a new message router.
func NewRouter(cfg RouterConfig) *Router {
	eh := cfg.ErrorHandler
	if eh == nil {
		eh = func(msg *Message, err error) {
			slog.Error("router error", "error", err)
		}
	}

	// Set up pipe error handler to delegate to router's error handler
	pipeConfig := cfg.PipeConfig
	pipeConfig.ErrorHandler = func(in any, err error) {
		msg := in.(*Message)
		eh(msg, err)
	}

	return &Router{
		handlers:     make(map[string]handlerEntry),
		errorHandler: eh,
		pipeConfig:   pipeConfig,
	}
}

// AddHandler registers a handler for its CE type.
// The optional Matcher in HandlerConfig is applied after type matching.
func (r *Router) AddHandler(h Handler, cfg HandlerConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.EventType()] = handlerEntry{handler: h, config: cfg}
	return nil
}

// Pipe routes messages to handlers and returns outputs.
// Signature matches pipe.Pipe[*Message, *Message] for composability.
func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
	p := pipe.NewProcessPipe(r.process, r.pipeConfig)
	return p.Pipe(ctx, in)
}

// handler returns the handler entry for the given CE type.
func (r *Router) handler(ceType string) (handlerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.handlers[ceType]
	return entry, ok
}

func (r *Router) process(ctx context.Context, msg *Message) ([]*Message, error) {
	ceType, _ := msg.Attributes["type"].(string)

	entry, ok := r.handler(ceType)
	if !ok {
		return nil, ErrNoHandler
	}

	if entry.config.Matcher != nil && !entry.config.Matcher.Match(msg) {
		return nil, ErrHandlerRejected
	}

	outputs, err := entry.handler.Handle(ctx, msg)
	if err != nil {
		return nil, err
	}

	msg.Ack()
	return outputs, nil
}
