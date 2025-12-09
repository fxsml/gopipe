package message

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
)

var (
	// ErrInvalidMessagePayload indicates message payload marshal or unmarshal failure.
	ErrInvalidMessagePayload = fmt.Errorf("invalid message payload")
)

// RouterConfig configures message routing behavior.
type RouterConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *gopipe.RetryConfig
	Recover     bool
}

// HandlerFunc is the core handler function that processes a message and returns output messages.
//
// This is the fundamental building block for message processing in the router.
// It receives a message and returns zero or more output messages, or an error.
//
// Example:
//
//	handlerFunc := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
//	    // Process the message
//	    result := processMessage(msg)
//	    // Return output messages
//	    return []*message.Message{result}, nil
//	}
type HandlerFunc func(ctx context.Context, msg *Message) ([]*Message, error)

// HandlerMiddleware wraps a HandlerFunc to add additional behavior.
//
// Middleware enables cross-cutting concerns like logging, metrics, tracing,
// authentication, and error handling at the router level without modifying handlers.
//
// Middleware is applied in the order added: for middlewares A, B, C,
// the execution flow is A→B→C→handler.
//
// Example:
//
//	loggingMiddleware := func(next message.HandlerFunc) message.HandlerFunc {
//	    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
//	        log.Printf("Processing message")
//	        result, err := next(ctx, msg)
//	        log.Printf("Completed: error=%v", err != nil)
//	        return result, err
//	    }
//	}
//
// See middleware_examples.md for comprehensive examples.
type HandlerMiddleware func(next HandlerFunc) HandlerFunc

// Router dispatches messages to registered handlers based on property matching.
type Router struct {
	handlers   []Handler
	middleware []HandlerMiddleware
	config     RouterConfig
}

// NewRouter creates a router with the given configuration and handlers.
func NewRouter(config RouterConfig, handlers ...Handler) *Router {
	return &Router{
		handlers: handlers,
		config:   config,
	}
}

// AddHandler appends a handler to the router's handler list.
func (r *Router) AddHandler(handler Handler) {
	r.handlers = append(r.handlers, handler)
}

// AddMiddleware appends middleware to the router's middleware chain.
// Middleware is executed in the order added.
//
// Example:
//
//	router.AddMiddleware(loggingMiddleware, metricsMiddleware)
//	// Execution order: loggingMiddleware → metricsMiddleware → handler
func (r *Router) AddMiddleware(m ...HandlerMiddleware) {
	r.middleware = append(r.middleware, m...)
}

// Start processes incoming messages through matched handlers and returns output messages.
func (r *Router) Start(ctx context.Context, msgs <-chan *Message) <-chan *Message {
	// Core handler logic: find and execute matching handler
	coreHandler := func(ctx context.Context, msg *Message) ([]*Message, error) {
		for _, h := range r.handlers {
			if h.Match(msg.Properties) {
				return h.Handle(ctx, msg)
			}
		}
		err := fmt.Errorf("no handler matched")
		msg.Nack(err)
		return nil, err
	}

	// Apply middleware chain
	handle := applyMiddleware(coreHandler, r.middleware...)

	opts := []gopipe.Option[*Message, *Message]{
		gopipe.WithLogConfig[*Message, *Message](gopipe.LogConfig{
			MessageSuccess: "Processed messages",
			MessageFailure: "Failed to process messages",
			MessageCancel:  "Canceled processing messages",
		}),
		gopipe.WithMetadataProvider[*Message, *Message](func(msg *Message) gopipe.Metadata {
			metadata := gopipe.Metadata{}
			if id, ok := msg.Properties.ID(); ok {
				metadata[PropID] = id
			}
			if corr, ok := msg.Properties.CorrelationID(); ok {
				metadata[PropCorrelationID] = corr
			}
			return metadata
		}),
	}
	if r.config.Recover {
		opts = append(opts, gopipe.WithRecover[*Message, *Message]())
	}
	if r.config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[*Message, *Message](r.config.Concurrency))
	}
	if r.config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[*Message, *Message](r.config.Timeout))
	}
	if r.config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[*Message, *Message](*r.config.Retry))
	}

	return gopipe.NewProcessPipe(handle, opts...).Start(ctx, msgs)
}

// applyMiddleware applies middleware to a handler function.
// Middleware is applied in the order provided: for middlewares A, B, C,
// the execution flow is A→B→C→handler.
func applyMiddleware(handler HandlerFunc, middleware ...HandlerMiddleware) HandlerFunc {
	// Apply middleware in reverse order so that the first middleware is the outermost
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}

// Handler processes messages matching specific properties.
type Handler interface {
	Handle(ctx context.Context, msg *Message) ([]*Message, error)
	Match(prop Properties) bool
}

type handler struct {
	handle func(ctx context.Context, msg *Message) ([]*Message, error)
	match  func(prop Properties) bool
}

func (h *handler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	return h.handle(ctx, msg)
}

func (h *handler) Match(prop Properties) bool {
	return h.match(prop)
}

// NewHandler creates a handler from message processing and matching functions.
func NewHandler(
	handle func(ctx context.Context, msg *Message) ([]*Message, error),
	match func(prop Properties) bool,
) Handler {
	return &handler{
		handle: handle,
		match:  match,
	}
}
