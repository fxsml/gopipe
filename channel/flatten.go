package channel

// Flatten unpacks slices from in, sending each element individually.
// The returned channel is closed after in is closed.
func Flatten[T any](
	in <-chan []T,
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for batch := range in {
			for _, val := range batch {
				out <- val
			}
		}
	}()

	return out
}
