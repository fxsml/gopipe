package gopipeline

import (
	"context"
	"log"
	"os"
	"time"
)

// ErrorHandler is a function type for handling errors that occur during processing.
// The first parameter is the value that caused the error, and the second is the error itself.
type ErrorHandler func(any, error)

// Create a logger with timestamps (date and time)
var logger = log.New(os.Stderr, "", log.LstdFlags)

var defaultConfig = config{
	concurrency: 1,
	buffer:      1,
	err: func(_ any, err error) {
		logger.Printf("Error: %v", err)
	},
	ctx: func(ctx context.Context) (context.Context, context.CancelFunc) {
		return ctx, func() {}
	},
}

// config holds the configuration options for pipeline stages.
type config struct {
	concurrency int
	buffer      int
	err         ErrorHandler
	ctx         func(context.Context) (context.Context, context.CancelFunc)
}

// Option is a function type for configuring pipeline stages.
type Option func(*config)

// WithConcurrency sets the number of concurrent workers for processing items.
//
// If concurrency is less than or equal to zero, the default value is kept.
func WithConcurrency(concurrency int) Option {
	return func(cfg *config) {
		if concurrency > 0 {
			cfg.concurrency = concurrency
		}
	}
}

// WithBuffer sets the buffer size for the output channel.
//
// If buffer is less than or equal to zero, the default value is kept.
func WithBuffer(buffer int) Option {
	return func(cfg *config) {
		if buffer > 0 {
			cfg.buffer = buffer
		}
	}
}

// WithProcessTimeout sets a timeout for each process operation.
//
// If timeout is less than or equal to zero, the default context behavior is kept.
func WithProcessTimeout(timeout time.Duration) Option {
	return func(cfg *config) {
		if timeout > 0 {
			cfg.ctx = func(ctx context.Context) (context.Context, context.CancelFunc) {
				return context.WithTimeout(ctx, timeout)
			}
		}
	}
}

// WithErrorHandler sets a custom error handler for processing errors.
//
// If the provided handler is nil, the default error handler is kept.
func WithErrorHandler(handler ErrorHandler) Option {
	return func(cfg *config) {
		if handler != nil {
			cfg.err = handler
		}
	}
}
