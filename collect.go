package gopipe

import (
	"time"
)

// Collect groups values from in into batches, sending each batch when
// either maxSize is reached or maxDuration elapses. The returned channel
// is closed after in is closed. maxSize and maxDuration must be > 0.
func Collect[T any](
	in <-chan T,
	maxSize int,
	maxDuration time.Duration,
) <-chan []T {
	out := make(chan []T)
	batch := make([]T, 0, maxSize)
	ticker := time.NewTicker(maxDuration)

	go func() {
		defer func() {
			ticker.Stop()
			if len(batch) > 0 {
				out <- batch
			}
			close(out)
		}()

		for {
			select {
			case val, ok := <-in:
				if !ok {
					return
				}
				batch = append(batch, val)
				if len(batch) >= maxSize {
					out <- batch
					batch = make([]T, 0, maxSize)
				}
			case <-ticker.C:
				if len(batch) > 0 {
					out <- batch
					batch = make([]T, 0, maxSize)
				}
			}
		}
	}()

	return out
}
