package channel

// Broadcast duplicates values from in to n output channels.
// All receivers get each value. Blocks if any receiver is slow.
// The returned channels are closed after in is closed. n must be >= 0.
func Broadcast[T any](
	in <-chan T,
	n int,
) []<-chan T {
	outs := make([]chan T, n)
	outsRO := make([]<-chan T, n)

	for i := range outs {
		outs[i] = make(chan T)
		outsRO[i] = outs[i]
	}

	go func() {
		defer func() {
			for _, out := range outs {
				close(out)
			}
		}()

		for val := range in {
			for _, out := range outs {
				out <- val
			}
		}
	}()

	return outsRO
}
