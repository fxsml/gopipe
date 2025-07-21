package gopipe

import (
	"context"
	"sync"
)

// Merge creates a fan-in pattern that combines multiple input channels into a single 
// buffered output channel.
//
// Merge takes a context for cancellation, a buffer size for the output channel, and
// a variable number of input channels. It returns a single output channel that receives
// values from all input channels as they arrive.
//
// This function is useful for scenarios where data from multiple sources needs to be
// processed in a single pipeline, such as:
//   - Combining results from parallel processing steps
//   - Aggregating data from multiple sources
//   - Creating a single stream from multiple producers
//
// Features:
//   - Concurrent reading from all input channels
//   - Non-blocking writes to the output channel (returns early if context is cancelled)
//   - Automatic cleanup with proper channel closing when all inputs are exhausted
//   - Configurable buffer size for the output channel
//
// Example:
//
//	input1 := make(chan int)
//	input2 := make(chan int)
//	input3 := make(chan int)
//	
//	// Combined will receive values from all three input channels
//	combined := gopipe.Merge(ctx, 10, input1, input2, input3)
//
// The output channel is closed when all input channels are closed or the context is cancelled.
func Merge[T any](ctx context.Context, buffer int, ins ...<-chan T) <-chan T {
	out := make(chan T, buffer)
	var wg sync.WaitGroup
	wg.Add(len(ins))

	for _, in := range ins {
		go func(in <-chan T) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case v, ok := <-in:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
		}(in)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
