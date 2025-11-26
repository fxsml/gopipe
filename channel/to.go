package channel

// ToSlice collects all values from the input channel into a slice.
// It blocks until the input channel is closed.
func ToSlice[T any](
	in <-chan T,
) []T {
	var slice []T
	for val := range in {
		slice = append(slice, val)
	}
	return slice
}
