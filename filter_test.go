package gopipe

import (
	"context"
	"testing"
)

type filterFunc[T any] func(<-chan T, func(T) bool) <-chan T

var filterFuncs = map[string]filterFunc[int]{
	"Filter": Filter[int],
	"FilterPipe": func(in <-chan int, handle func(int) bool) <-chan int {
		handlePipe := func(_ context.Context, val int) (bool, error) {
			return handle(val), nil
		}
		return NewFilterPipe(
			handlePipe,
		).Start(context.Background(), in)
	},
}

func TestFilter_Even(t *testing.T) {
	for name, filterFunc := range filterFuncs {
		t.Run(name, func(t *testing.T) {
			in := make(chan int)
			out := filterFunc(in, func(v int) bool { return v%2 == 0 })

			go func() {
				for i := 1; i <= 5; i++ {
					in <- i
				}
				close(in)
			}()

			var got []int
			for v := range out {
				got = append(got, v)
			}

			expected := []int{2, 4}
			if len(got) != len(expected) {
				t.Fatalf("expected %v, got %v", expected, got)
			}
			for i := range expected {
				if got[i] != expected[i] {
					t.Fatalf("at %d expected %d got %d", i, expected[i], got[i])
				}
			}
		})
	}
}

func TestFilter_AllFalse(t *testing.T) {
	for name, filterFunc := range filterFuncs {
		t.Run(name, func(t *testing.T) {
			in := make(chan int, 2)
			in <- 1
			in <- 3
			close(in)

			out := filterFunc(in, func(v int) bool { return false })

			var got []int
			for v := range out {
				got = append(got, v)
			}

			if len(got) != 0 {
				t.Fatalf("expected no values, got %v", got)
			}
		})
	}
}

func TestFilter_Closure(t *testing.T) {
	for name, filterFunc := range filterFuncs {
		t.Run(name, func(t *testing.T) {
			in := make(chan int, 1)
			in <- 42
			close(in)

			out := filterFunc(in, func(v int) bool { return true })

			// read the value and ensure the channel is closed afterwards
			if v, ok := <-out; !ok || v != 42 {
				t.Fatalf("expected value 42, got %v (ok=%v)", v, ok)
			}
			if _, ok := <-out; ok {
				t.Fatalf("expected output to be closed")
			}
		})
	}
}
