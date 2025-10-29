package channel

// Drain consumes and discards all values from in.
// The returned channel is closed after in is closed and all values are consumed.
func Drain[T any](
	in <-chan T,
) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		for range in {
		}
	}()

	return done
}
