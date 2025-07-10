package gopipeline

import "context"

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
