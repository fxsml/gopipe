package gopipe

import (
	"context"
	"fmt"
	"sync"
)

var ErrRouteIndexOutOfRange = fmt.Errorf("gopipe: route index")

// RouteFunc determines which output channel an item should be sent to based on the item's value.
// It returns an integer index representing the target output channel.
type RouteFunc[T any] func(item T) int

// Route distributes items from the input channel to multiple output channels based on the route function.
// The route function should return an index between 0 and numOutputs-1, indicating which output channel
// should receive the item. If the route function returns an index outside this range, the item is discarded
// and an error is reported via the error handler.
//
// Route is useful for implementing content-based routing in a pipeline, where items need to be
// directed to different processing paths based on their content or characteristics.
//
// The function supports concurrent processing through the WithConcurrency option, allowing
// multiple routing operations to occur in parallel. The WithBuffer option controls the buffer
// size of each output channel.
//
// The goroutine(s) started by Route will exit automatically when either:
//   - The input channel is closed
//   - The provided context is cancelled
//
// All output channels will be closed when processing completes.
//
// Example:
//
//	// Route users to different channels based on their status
//	userChan := make(chan User)
//	outputs := gopipe.Route(ctx, userChan, func(user User) int {
//	    switch user.Status {
//	    case "active":
//	        return 0
//	    case "pending":
//	        return 1
//	    case "inactive":
//	        return 2
//	    default:
//	        return -1 // Will be discarded
//	    }
//	}, 3, gopipe.WithBuffer(10))
//
//	// Now we can process each type separately
//	activeUsers := outputs[0]
//	pendingUsers := outputs[1]
//	inactiveUsers := outputs[2]
//
// Returns a slice of output channels that will receive items based on the route function results.
func Route[T any](
	ctx context.Context,
	in <-chan T,
	routeFunc RouteFunc[T],
	n int,
	opts ...Option,
) []<-chan T {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	outs := make([]chan T, n)
	outsReadOnly := make([]<-chan T, n)

	for i := range outs {
		outs[i] = make(chan T, cfg.buffer)
		outsReadOnly[i] = outs[i]
	}

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

					idx := routeFunc(val)

					if idx < 0 || idx >= n {
						cfg.err(val, fmt.Errorf("%w %d out of range [0, %d]", ErrRouteIndexOutOfRange, idx, n))
					} else {
						select {
						case <-ctx.Done():
							return
						case outs[idx] <- val:
						}
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		for i := range outs {
			close(outs[i])
		}
	}()

	return outsReadOnly
}
