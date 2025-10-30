package channel

// FromSlice sends each element of slice into the returned channel.
// The returned channel is closed after all values have been sent.
func FromSlice[T any](
	slice []T,
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for _, val := range slice {
			out <- val
		}
	}()

	return out
}

// FromValues sends each value into the returned channel.
// The returned channel is closed after all values have been sent.
func FromValues[T any](
	values ...T,
) <-chan T {
	return FromSlice(values)
}

// FromRange returns a channel that emits a sequence of integers.
// Usage:
//
//	FromRange(to)              // emits 0, 1, ..., to-1 (step=1)
//	FromRange(from, to)        // emits from, from+1, ..., to-1 (step=1)
//	FromRange(from, to, step)  // emits from, from+step, ... < to
//
// If step > 0, emits ascending values: from, from+step, ... < to
// If step < 0, emits descending values: from, from+step, ... > to
// If step == 0, panics.
//
// The channel is closed after all values have been sent.
// Panics if called with zero or more than three arguments.
func FromRange(
	i ...int,
) <-chan int {
	var from, to, step int
	switch len(i) {
	case 0:
		panic("FromRange requires at least 1 integer argument")
	case 1:
		from = 0
		to = i[0]
		step = 1
	case 2:
		from = i[0]
		to = i[1]
		step = 1
	case 3:
		from = i[0]
		to = i[1]
		step = i[2]
	default:
		panic("FromRange accepts at most 3 integer arguments")
	}
	out := make(chan int)

	if step == 0 {
		panic("FromRange step must not be null")
	}

	go func() {
		defer close(out)
		if step < 0 {
			for j := from; j > to; j += step {
				out <- j
			}
			return
		}
		for j := from; j < to; j += step {
			out <- j
		}
	}()

	return out
}

// FromFunc generates values by repeatedly calling handle.
// When handle returns false, the generation stops. The value
// returned along with false is ignored.
// The returned channel is closed when handle returns false.
func FromFunc[T any](
	handle func() (T, bool),
) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for {
			val, ok := handle()
			if !ok {
				break
			}
			out <- val
		}
	}()

	return out
}
