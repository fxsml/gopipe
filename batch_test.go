package gopipe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func Test_processBatchResult_successAndError(t *testing.T) {
	proc := func(ctx context.Context, batch []int) (BatchResult[int, string], error) {
		res := NewBatchResult[int, string](len(batch))
		for _, v := range batch {
			if v%2 == 0 {
				res.AddFailure(v, errors.New("bad"))
			} else {
				res.AddSuccess(fmt.Sprintf("v:%d", v))
			}
		}
		return res, nil
	}

	var cancelled [][]int
	cancel := func(in []int, err error) {
		cancelled = append(cancelled, in)
	}

	f := processBatchResult(proc, cancel)

	out, err := f(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// expect outputs for 1 and 3 (odd numbers)
	if !reflect.DeepEqual(out, []string{"v:1", "v:3"}) {
		t.Fatalf("unexpected outputs: %v", out)
	}

	// expect cancel called once for the even value 2
	if len(cancelled) != 1 || len(cancelled[0]) != 1 || cancelled[0][0] != 2 {
		t.Fatalf("unexpected cancelled calls: %v", cancelled)
	}
}

func Test_processBatchResult_multipleErrors(t *testing.T) {
	proc := func(ctx context.Context, batch []int) (BatchResult[int, string], error) {
		res := NewBatchResult[int, string](len(batch))
		res.AddFailure(batch[0], errors.New("err1"))
		res.AddFailure(batch[1], errors.New("err2"))
		return res, nil
	}

	var cancelled [][]int
	cancel := func(in []int, err error) {
		cancelled = append(cancelled, in)
	}

	f := processBatchResult(proc, cancel)
	out, err := f(context.Background(), []int{7, 9})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no outputs, got %v", out)
	}
	if len(cancelled) != 2 || cancelled[0][0] != 7 || cancelled[1][0] != 9 {
		t.Fatalf("unexpected cancelled sequence: %v", cancelled)
	}
}

func Test_processBatchResult_processErrorPropagates(t *testing.T) {
	sentinel := errors.New("process failed")
	proc := func(ctx context.Context, batch []int) (BatchResult[int, string], error) {
		return nil, sentinel
	}

	var cancelled [][]int
	cancel := func(in []int, err error) {
		cancelled = append(cancelled, in)
	}

	f := processBatchResult(proc, cancel)
	_, err := f(context.Background(), []int{1, 2})
	if err == nil {
		t.Fatalf("expected error but got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cancelled) != 0 {
		t.Fatalf("cancel should not be called when process returns error, got %v", cancelled)
	}
}

func Test_Batch_basic(t *testing.T) {
	in := make(chan int)

	proc := func(ctx context.Context, batch []int) (BatchResult[int, string], error) {
		res := NewBatchResult[int, string](len(batch))
		for _, v := range batch {
			if v == 2 {
				res.AddFailure(v, errors.New("bad"))
			} else {
				res.AddSuccess(fmt.Sprintf("%d", v*10))
			}
		}
		return res, nil
	}

	var cancelled [][]int
	cancel := func(in []int, err error) {
		cancelled = append(cancelled, in)
	}

	out := Batch(context.Background(), in, proc, cancel, 3, time.Second)

	go func() {
		in <- 1
		in <- 2
		in <- 3
		close(in)
	}()

	var got []string
	for v := range out {
		got = append(got, v)
	}

	if !reflect.DeepEqual(got, []string{"10", "30"}) {
		t.Fatalf("unexpected outputs: %v", got)
	}
	if len(cancelled) != 1 || cancelled[0][0] != 2 {
		t.Fatalf("unexpected cancelled calls: %v", cancelled)
	}
}
