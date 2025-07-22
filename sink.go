package gopipe

import (
	"context"
	"fmt"
)

var ErrSink = fmt.Errorf("gopipe: sink")

// SinkHandler is a function that processes input values in a Sink operation.
// It returns an error if the processing fails.
type SinkHandler[In any] func(context.Context, In) error

// Sink processes values from an input channel using the provided handler function.
// Unlike Drain, which simply discards values, Sink applies meaningful processing
// to each value through the handler function.
//
// Sink is typically used as the final stage in a data processing pipeline where
// you need to perform operations on each value but don't need to produce any
// further output (e.g., saving to a database, making API calls, or logging).
//
// The function supports concurrent processing through the WithConcurrency option,
// allowing multiple handlers to process items in parallel. Note that the WithBuffer
// option has no effect on Sink as it doesn't produce an output channel.
//
// The goroutine(s) started by Sink will exit automatically when either:
//   - The input channel is closed
//   - The provided context is cancelled
//
// Example:
//
//	// Create a pipeline that processes and saves data
//	inputChan := make(chan User)
//	gopipe.Sink(ctx, inputChan, func(ctx context.Context, user User) error {
//	    // Save the user to database
//	    return db.SaveUser(ctx, user)
//	}, gopipe.WithConcurrency(5))
//
// Note that Sink does not block until all items are processed. It starts
// goroutines to handle the processing asynchronously.
func Sink[In any](
	ctx context.Context,
	in <-chan In,
	handler SinkHandler[In],
	opts ...Option,
) {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	for range cfg.concurrency {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					ctxSink, cancel := cfg.ctx(ctx)
					if err := handler(ctxSink, val); err != nil {
						cfg.err(val, fmt.Errorf("%w: %w", ErrSink, err))
					}
					cancel()
				}
			}
		}()
	}
}
