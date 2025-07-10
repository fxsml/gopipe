package gopipeline

import (
	"context"
	"fmt"
	"os"
	"time"
)

type ErrorHandler func(any, error)

var defaultConfig = config{
	concurrency: 1,
	buffer:      1,
	err: func(_ any, err error) {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	},
	ctx: func(ctx context.Context) (context.Context, context.CancelFunc) {
		return ctx, func() {}
	},
}

type config struct {
	concurrency int
	buffer      int
	err         ErrorHandler
	ctx         func(context.Context) (context.Context, context.CancelFunc)
}

type Option func(*config)

func WithConcurrency(concurrency int) Option {
	if concurrency <= 0 {
		panic("concurrency must be greater than 0")
	}

	return func(cfg *config) {
		cfg.concurrency = concurrency
	}
}

func WithBuffer(buffer int) Option {
	if buffer <= 0 {
		panic("buffer size must be greater than 0")
	}

	return func(cfg *config) {
		cfg.buffer = buffer
	}
}

func WithProcessTimeout(timeout time.Duration) Option {
	if timeout <= 0 {
		panic("timeout must be greater than 0")
	}

	return func(cfg *config) {
		cfg.ctx = func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, timeout)
		}
	}
}

func WithErrorHandler(err ErrorHandler) Option {
	if err == nil {
		panic("error handler cannot be nil")
	}

	return func(cfg *config) {
		cfg.err = err
	}
}
