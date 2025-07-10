package gopipeline

import "context"

// Broadcast sends all values from the input channel to each of the returned buffered output channels.
func Broadcast[T any](ctx context.Context, n, buffer int, in <-chan T) []<-chan T {
	outs := make([]chan T, n)
	for i := range n {
		outs[i] = make(chan T, buffer)
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

	result := make([]<-chan T, n)
	for i, ch := range outs {
		result[i] = ch
	}
	return result
}
