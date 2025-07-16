package gopipeline

import (
	"context"
	"fmt"
	"sync"
)

// ErrFilter is returned when a filter function returns an error.
var ErrFilter = fmt.Errorf("gopipeline: filter")

// FilterHandler is a function type that evaluates whether an input value should be included
// in the output or discarded. It returns a boolean indicating whether to keep the item (true)
// or discard it (false), and an error if the evaluation fails.
type FilterHandler[In any] func(context.Context, In) (bool, error)

// Filter creates a pipeline stage that filters input values from the input channel
// using the provided handler function and sends only the matching values to the output channel.
//
// Filter takes a context for cancellation, an input channel, a filter handler function, and optional
// configuration options. It returns an output channel where only the values that pass the filter
// will be sent.
//
// The filter handler function is called for each input value. It should return true if the value
// should be passed to the output channel, or false if it should be filtered out. If the handler
// returns an error, the error is passed to the configured error handler and the value is not sent
// to the output channel.
//
// Filter features:
//   - Concurrent filtering with configurable number of workers
//   - Configurable output channel buffer size
//   - Custom error handling
//   - Context-based cancellation
//   - Optional processing timeouts
//
// Example:
//
//	out := gopipeline.Filter(ctx, inputChan, func(ctx context.Context, v int) (bool, error) {
//	    return v%2 == 0, nil // Only keep even numbers
//	}, gopipeline.WithConcurrency(3))
//
// The output channel is automatically closed when all inputs have been processed
// or when the context is cancelled.
func Filter[T any](
	ctx context.Context,
	in <-chan T,
	handler FilterHandler[T],
	opts ...Option,
) <-chan T {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	out := make(chan T, cfg.buffer)

	var wg sync.WaitGroup
	wg.Add(cfg.concurrency)
	for range cfg.concurrency {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					ctxFilter, cancel := cfg.ctx(ctx)
					if ok, err := handler(ctxFilter, val); err != nil {
						cfg.err(val, fmt.Errorf("%w: %w", ErrFilter, err))
					} else if ok {
						select {
						case out <- val:
						case <-ctx.Done():
						}
					}
					cancel()
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
