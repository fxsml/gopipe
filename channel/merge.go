package channel

import (
	"sync"
)

// Merge combines multiple input channels into a single output channel.
// All values from all input channels are forwarded to the output channel.
// The returned channel is closed after all inputs are closed.
func Merge[T any](
	ins ...<-chan T,
) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	wg.Add(len(ins))

	for _, in := range ins {
		go func(in <-chan T) {
			defer wg.Done()
			for val := range in {
				out <- val
			}
		}(in)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
