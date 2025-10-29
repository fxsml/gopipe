package channel

// Process maps each value from in to zero or more values using handle.
// The returned channel is closed after in is closed.
func Process[In, Out any](
	in <-chan In,
	handle func(In) []Out,
) <-chan Out {
	out := make(chan Out)

	go func() {
		defer close(out)
		for val := range in {
			for _, item := range handle(val) {
				out <- item
			}
		}
	}()

	return out
}
