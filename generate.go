package gopipe

import (
	"context"
)

// Generator is a function that produces a stream of values when invoked.
// It takes a context for cancellation and returns a channel that will receive the generated values.
type Generator[Out any] func(context.Context) <-chan Out

// Generate creates a pipeline source that produces items on demand, triggered by signals.
// Unlike other sources that continuously produce items, Generate only starts producing items
// when it receives a signal through the provided signal channel.
//
// Generate is useful for scenarios where data production should be controlled by external events, such as:
//   - Polling a resource at specific intervals or on-demand
//   - Processing batches of data in response to user actions
//   - Coordinating multiple data generation phases
//   - Implementing backpressure mechanisms by controlling when to generate more data
//
// When a signal is received, the generator function is invoked to create a stream of items.
// After all items are processed (when the generator's output channel is closed), a notification
// is sent on the done channel using the same signal value that triggered the generation.
//
// The goroutine started by Generate will exit when either:
//   - The signal channel is closed
//   - The provided context is cancelled
//
// Example:
//
//	// Create signal and notification channels
//	signalCh := make(chan struct{}, 1)
//	doneCh := make(chan struct{}, 1)
//
//	// Create a generator function
//	generator := func(ctx context.Context) <-chan int {
//	    ch := make(chan int, 5)
//	    go func() {
//	        defer close(ch)
//	        for i := 0; i < 5; i++ {
//	            ch <- i
//	        }
//	    }()
//	    return ch
//	}
//
//	// Start the generator
//	out := gopipe.Generate(ctx, generator, signalCh, doneCh)
//
//	// Send a signal to start generation
//	signalCh <- struct{}{}
//
//	// Wait for generation to complete
//	<-doneCh
//
// Returns a channel that will receive all values produced by the generator function.
func Generate[Out any, S any](
	ctx context.Context,
	generator Generator[Out],
	signal <-chan S,
	done chan<- S,
	opts ...Option,
) <-chan Out {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	signalDone := func(cancel func(), s S) {
		cancel()
		select {
		case done <- s:
		default:
		}
	}

	out := make(chan Out, cfg.buffer)

	// Start the generation process
	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case s, ok := <-signal:
				if !ok {
					return
				}

				ctxGenerator, cancel := cfg.ctx(ctx)
				inCh := generator(ctxGenerator)

				// Forward all generated items to the output channel
				for item := range inCh {
					select {
					case out <- item:
					case <-ctx.Done():
						signalDone(cancel, s)
						return
					}
				}
				signalDone(cancel, s)
			}
		}
	}()

	return out
}
