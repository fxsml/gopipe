package gopipeline

import "context"

// Drain creates a non-blocking consumer that continuously reads and discards values from
// an input channel until the channel is closed or the context is cancelled.
//
// Drain is useful in scenarios where you need to ensure a channel is being consumed
// to prevent blocking in upstream pipeline stages, but don't need to process the values.
// This function can be used to safely discard unwanted output from other pipeline stages.
//
// The goroutine started by Drain will exit automatically when either:
//   - The input channel is closed
//   - The provided context is cancelled
//
// Example:
//
//	// Create a pipeline where we only care about side effects in the Process stage
//	inputChan := make(chan int)
//	processed := gopipeline.Process(ctx, inputChan, func(ctx context.Context, v int) (int, error) {
//	    // Do some work with side effects
//	    log.Printf("Processing value: %d", v)
//	    return v, nil
//	})
//
//	// Drain the processed output since we don't need the values
//	gopipeline.Drain(ctx, processed)
//
// Note that Drain does not wait for the channel to be closed before returning.
// It starts a goroutine that handles the draining operation asynchronously.
func Drain[T any](ctx context.Context, in <-chan T) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-in:
				if !ok {
					return
				}
			}
		}
	}()
}
