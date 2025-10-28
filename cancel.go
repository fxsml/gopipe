package gopipe

import (
	"context"
	"sync"
)

// Cancel forwards values from in until ctx is done or in is closed.
// If cancelled, remaining values trigger cancel callback with ErrCancel
// wrapping ctx.Err().
func Cancel[T any](
	ctx context.Context,
	in <-chan T,
	cancel func(T, error),
) <-chan T {
	out := make(chan T)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer func() {
			close(out)
			wg.Done()
		}()

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
				select {
				case <-ctx.Done():
					cancel(val, newErrCancel(ctx.Err()))
					return
				case out <- val:
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		for val := range in {
			cancel(val, newErrCancel(ctx.Err()))
		}
	}()

	return out
}
