package gopipe

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCancel_Drains(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	var mu sync.Mutex
	var vals []int
	var errs []error

	cancelFn := func(v int, err error) {
		mu.Lock()
		vals = append(vals, v)
		errs = append(errs, err)
		mu.Unlock()
	}

	// cancel the context before starting; Cancel should drain and call cancelFn
	cancelCtx()
	_ = Cancel(ctx, in, cancelFn)

	// wait for drain to finish
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		l := len(vals)
		mu.Unlock()
		if l == 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for cancelFn to be called; got %d calls", l)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// verify values and errors
	mu.Lock()
	defer mu.Unlock()
	if len(vals) != 3 {
		t.Fatalf("expected 3 cancelled values, got %d", len(vals))
	}
	expected := map[int]bool{1: true, 2: true, 3: true}
	for _, v := range vals {
		if !expected[v] {
			t.Fatalf("unexpected cancelled value: %d", v)
		}
	}
	for _, e := range errs {
		if !errors.Is(e, ErrCancel) {
			t.Fatalf("expected ErrCancel, got %v", e)
		}
	}
}

func TestCancel_InFlight(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	in := make(chan int)

	var mu sync.Mutex
	var vals []int
	var errs []error

	cancelFn := func(v int, err error) {
		mu.Lock()
		vals = append(vals, v)
		errs = append(errs, err)
		mu.Unlock()
	}

	_ = Cancel(ctx, in, cancelFn)

	// send a value; the send will complete only after Cancel has received it,
	// and Cancel will then block trying to send it to the returned channel.
	sent := make(chan struct{})
	go func() {
		in <- 42
		close(sent)
		// close the input to allow drain goroutine to finish after cancellation
		close(in)
	}()

	// wait until the send completed (meaning Cancel has received the value)
	select {
	case <-sent:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for value to be received by Cancel")
	}

	// now cancel; Cancel should call cancelFn for the in-flight value
	cancelCtx()

	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		l := len(vals)
		mu.Unlock()
		if l >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for cancelFn for in-flight value")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(vals) == 0 || vals[0] != 42 {
		t.Fatalf("expected cancelled value 42, got %v", vals)
	}
	if !errors.Is(errs[0], ErrCancel) {
		t.Fatalf("expected ErrCancel, got %v", errs[0])
	}
}
