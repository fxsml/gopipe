package gopipe

import "context"

// Broadcast creates a fan-out pattern that duplicates all values from the input channel
// to multiple buffered output channels.
//
// Broadcast takes a context for cancellation, the number of output channels to create,
// a buffer size for those channels, and an input channel. It returns a slice of output
// channels, each receiving all values from the input channel.
//
// This function is useful for scenarios where multiple independent consumers need
// to process the same data stream, such as:
//   - Parallel processing with different transformations on the same data
//   - Sending the same data to multiple destinations
//   - Creating redundant processing paths for reliability
//
// Features:
//   - Non-blocking writes to output channels (returns early if context is cancelled)
//   - Proper cleanup with channel closing when input is exhausted
//   - Configurable buffer size for all output channels
//
// Example:
//
//	inputChan := make(chan int)
//	outputs := gopipe.Broadcast(ctx, 3, 10, inputChan)
//
//	// Each of outputs[0], outputs[1], and outputs[2] will receive all values from inputChan
//
// All output channels are closed when the input channel is closed or the context is cancelled.
func Broadcast[T any](ctx context.Context, n, buffer int, in <-chan T) []<-chan T {
	outs := make([]chan T, n)
	outsReadOnly := make([]<-chan T, n)

	for i := range outs {
		outs[i] = make(chan T, buffer)
		outsReadOnly[i] = outs[i]
	}

	go func() {
		defer func() {
			for _, out := range outs {
				close(out)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				for _, out := range outs {
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
		}
	}()

	return outsReadOnly
}
