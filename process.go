package gopipe

import (
	"context"
	"sync"
)

// ProcessFunc processes a single item with context awareness.
type ProcessFunc[In, Out any] func(context.Context, In) (Out, error)

// CancelFunc handles errors when processing fails.
type CancelFunc[In any] func(In, error)

// Process applies process function to each item from in concurrently.
// Successful results are sent to the returned channel; failures invoke cancel.
// Behavior is configurable with options. The returned channel is closed
// after processing finishes.
func Process[In, Out any](
	ctx context.Context,
	in <-chan In,
	process ProcessFunc[In, Out],
	cancel CancelFunc[In],
	opts ...Option,
) <-chan Out {
	c := defaultConfig()
	for _, opt := range opts {
		opt(&c)
	}

	out := make(chan Out, c.buffer)

	var wg sync.WaitGroup
	wg.Add(c.concurrency)
	for i := 0; i < c.concurrency; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok {
						return
					}
					processCtx, processCancel := c.newProcessCtx(ctx)
					if res, err := process(processCtx, val); err != nil {
						cancel(val, err)
					} else {
						out <- res
					}
					processCancel()
				}
			}
		}()
	}

	// Start draining as soon as the parent context is cancelled.
	go func() {
		<-ctx.Done()
		for val := range in {
			cancel(val, ctx.Err())
		}
	}()

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
