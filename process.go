package gopipe

import (
	"context"
	"sync"
	"time"
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

	ctx, ctxCancel := context.WithCancel(ctx)
	out := make(chan Out, c.buffer)

	m := c.metrics

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

					if m != nil {
						m.IncInFlight()
					}

					processCtx, processCancel := c.newProcessCtx(ctx)
					start := time.Now()
					if res, err := process(processCtx, val); err != nil {
						if m != nil {
							m.IncFailure()
							m.DecInFlight()
							m.ObserveProcessingDuration(time.Since(start))
						}
						cancel(val, err)
					} else {
						if m != nil {
							m.IncSuccess()
							m.DecInFlight()
							m.ObserveProcessingDuration(time.Since(start))
						}
						out <- res
						if m != nil {
							m.ObserveBufferSize(len(out))
						}
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
			if m != nil {
				m.IncCancelled()
			}
			cancel(val, ctx.Err())
		}
	}()

	go func() {
		wg.Wait()
		close(out)
		ctxCancel()
	}()

	return out
}
