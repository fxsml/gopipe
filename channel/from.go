package channel

import "context"

// FromSlice sends each element of slice into the returned channel.
// The returned channel is closed after all values have been sent.
func FromSlice[T any](
	slice []T,
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for _, val := range slice {
			out <- val
		}
	}()

	return out
}

// FromValues sends each value into the returned channel.
// The returned channel is closed after all values have been sent.
func FromValues[T any](
	values ...T,
) <-chan T {
	return FromSlice(values)
}

// FromFunc generates values by repeatedly calling the handle function
// until context cancellation.
func FromFunc[T any](
	ctx context.Context,
	handle func() T,
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			select {
			case <-ctx.Done():
				return
			case out <- handle():
			}
		}
	}()

	return out
}
