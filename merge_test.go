package gopipe_test

import (
	"context"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
)

func TestMerge_OutputClosedWhenAllInputsClosed(t *testing.T) {
	ctx := context.Background()
	in1 := make(chan int)
	in2 := make(chan int)
	out := gopipe.Merge(ctx, 2, in1, in2)
	close(in1)
	close(in2)

	time.Sleep(10 * time.Millisecond)
	select {
	case _, ok := <-out:
		if ok {
			t.Error("expected output channel to be closed")
		}
	default:
	}
}

func TestMerge_OutputReceivesAllValues(t *testing.T) {
	ctx := context.Background()
	in1 := make(chan int, 2)
	in2 := make(chan int, 2)
	in1 <- 1
	in1 <- 2
	in2 <- 3
	in2 <- 4
	close(in1)
	close(in2)
	out := gopipe.Merge(ctx, 4, in1, in2)
	var got []int
	for v := range out {
		got = append(got, v)
	}
	want := map[int]bool{1: true, 2: true, 3: true, 4: true}
	if len(got) != 4 {
		t.Errorf("expected 4 values, got %d", len(got))
	}
	for _, v := range got {
		if !want[v] {
			t.Errorf("unexpected value %d", v)
		}
		delete(want, v)
	}
	if len(want) != 0 {
		t.Errorf("missing values: %v", want)
	}
}

func TestMerge_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in1 := make(chan int)
	in2 := make(chan int)
	out := gopipe.Merge(ctx, 2, in1, in2)
	cancel()

	time.Sleep(10 * time.Millisecond)
	select {
	case _, ok := <-out:
		if !ok {
			// closed as expected
		} else {
			t.Errorf("expected channel to be closed after context cancel")
		}
	default:
		t.Errorf("expected channel to be closed after context cancel")
	}
}
