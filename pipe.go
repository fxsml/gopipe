package gopipe

import (
	"context"
	"sync"
)

func pipe[In, Out any](
	ctx context.Context,
	in <-chan In,
	proc Processor[In, Out],
	opts ...Option,
) <-chan Out {
	c := defaultConfig()
	for _, opt := range opts {
		opt(&c)
	}

	ctx, ctxCancel := context.WithCancel(ctx)
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
					if res, err := proc.Process(processCtx, val); err != nil {
						proc.Cancel(val, newErrFailure(err))
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
			proc.Cancel(val, newErrCancel(ctx.Err()))
		}
	}()

	go func() {
		wg.Wait()
		close(out)
		ctxCancel()
	}()

	return out
}
