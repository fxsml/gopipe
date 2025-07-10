package gopipeline

import (
	"context"
	"sync"
)

// Merge merges all provided input channels into a single buffered output channel.
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
