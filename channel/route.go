package channel

// Route directs values from in to n output channels based on handle's index.
// Values with an index outside [0,n) are ignored. All returned channels
// are closed after in is closed.
func Route[T any](
	in <-chan T,
	handle func(item T) int,
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
			for _, ch := range outs {
				close(ch)
			}
		}()

		for val := range in {
			idx := handle(val)
			if idx < 0 || idx >= n {
				continue
			}
			outs[idx] <- val
		}
	}()

	return outsRO
}
