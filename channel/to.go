package channel

// ToSlice collects all values from the input channel into a slice.
// The returned channel is closed after the slice is collected.
func ToSlice[T any](
	in <-chan T,
) <-chan []T {
	out := make(chan []T)

	go func() {
		defer close(out)
		var slice []T
		for val := range in {
			slice = append(slice, val)
		}
		out <- slice
	}()

	return out
}
