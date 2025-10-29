package channel

// Filter passes through values from in for which handle returns true.
// The returned channel is closed after in is closed.
func Filter[T any](
	in <-chan T,
	handle func(T) bool,
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for val := range in {
			if handle(val) {
				out <- val
			}
		}
	}()

	return out
}
