package gopipe

// Sink applies handle to each value from in.
// The returned channel is closed after in is closed and all values are processed.
func Sink[T any](
	in <-chan T,
	handle func(T),
) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		for val := range in {
			handle(val)
		}
	}()

	return done
}
