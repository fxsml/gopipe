package gopipeline

import (
	"context"
	"fmt"
	"sync"
)

// ErrProcess is returned when a process function returns an error.
var ErrProcess = fmt.Errorf("gopipeline: process")

// ProcessHandler is a function type that processes a single input value and returns
// an output value or an error. The function receives a context that can be used for
// cancellation or timeout control.
type ProcessHandler[In, Out any] func(context.Context, In) (Out, error)

// Process creates a pipeline stage that processes each input value from the input channel
// using the provided handler function and sends the results to the output channel.
//
// Process takes a context for cancellation, an input channel, a handler function, and optional
// configuration options. It returns an output channel where the processed values will be sent.
//
// The handler function is called for each input value. If the handler returns an error, the error
// is passed to the configured error handler and the value is not sent to the output channel.
//
// Process features:
//   - Concurrent processing with configurable number of workers
//   - Configurable output channel buffer size
//   - Custom error handling
//   - Context-based cancellation
//   - Optional processing timeouts
//
// Example:
//
//	out := gopipeline.Process(ctx, inputChan, func(ctx context.Context, v int) (int, error) {
//	    return v * 2, nil
//	}, gopipeline.WithConcurrency(5), gopipeline.WithBuffer(10))
//
// The output channel is automatically closed when all inputs have been processed
// or when the context is cancelled.
func Process[In, Out any](
	ctx context.Context,
	in <-chan In,
	handler ProcessHandler[In, Out],
	opts ...Option,
) <-chan Out {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	out := make(chan Out, cfg.buffer)

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
					ctxProcess, cancel := cfg.ctx(ctx)
					if res, err := handler(ctxProcess, val); err != nil {
						cfg.err(val, fmt.Errorf("%w: %w", ErrProcess, err))
					} else {
						select {
						case out <- res:
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
