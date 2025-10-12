package gopipe

import (
	"context"
	"sync"
	"sync/atomic"
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
	c := newConfig(opts)

	return run(ctx, in, func(ctx context.Context, v In, _ *Metrics) (Out, error) {
		return process(ctx, v)
	}, cancel, c)
}

func run[In, Out any](
	ctx context.Context,
	in <-chan In,
	process func(context.Context, In, *Metrics) (Out, error),
	cancel CancelFunc[In],
	c config,
) <-chan Out {
	ctx, ctxCancel := context.WithCancel(ctx)
	out := make(chan Out, c.buffer)

	mc := c.metricsCollector
	inFlight := &atomic.Int32{}

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

					var m *Metrics
					if mc != nil {
						m = newProcessMetric(inFlight, len(in))
					}

					processCtx, processCancel := c.newProcessCtx(ctx)
					if res, err := process(processCtx, val, m); err != nil {
						if mc != nil {
							m.failure(inFlight, len(out))
						}
						cancel(val, err)
					} else {
						if mc != nil {
							m.success(inFlight, len(out))
						}
						out <- res
					}

					if mc != nil {
						mc.collect(m)
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
			var m *Metrics
			if mc != nil {
				m = newCancelMetric(len(in), len(out))
			}

			cancel(val, ctx.Err())

			if mc != nil {
				mc.collect(m)
			}
		}
		if mc != nil {
			mc.Done()
		}
	}()

	go func() {
		wg.Wait()
		close(out)
		ctxCancel()
	}()

	return out
}
