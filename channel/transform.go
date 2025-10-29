package channel

// Transform applies handle to each value from in and sends the result to the
// returned channel. The returned channel is closed after in is closed.
func Transform[In, Out any](
	in <-chan In,
	handle func(In) Out,
) <-chan Out {
	out := make(chan Out)

	go func() {
		defer close(out)
		for val := range in {
			out <- handle(val)
		}
	}()

	return out
}
