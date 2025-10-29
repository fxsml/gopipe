package channel

// Buffer returns a buffered channel with the specified size.
// The returned channel is closed after in is closed.
func Buffer[T any](
	in <-chan T,
	size int,
) <-chan T {
	out := make(chan T, size)

	go func() {
		defer close(out)
		for val := range in {
			out <- val
		}
	}()

	return out
}
