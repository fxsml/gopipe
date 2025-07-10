package gopipeline_test

import (
	"context"
	"testing"

	. "github.com/fxsml/gopipeline"
)

func TestFilter_Basic(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 6)
	for i := 1; i <= 6; i++ {
		in <- i
	}
	close(in)

	handler := func(ctx context.Context, v int) (bool, error) {
		return v%2 == 0, nil // keep even numbers
	}

	out := Filter(ctx, in, handler)
	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	expected := []int{2, 4, 6}
	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("at %d: expected %d, got %d", i, expected[i], v)
		}
	}
}
