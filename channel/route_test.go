package channel

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRoute_BasicRouting(t *testing.T) {
	in := make(chan int)
	outs := Route(in, func(v int) int { return v % 3 }, 3)

	go func() {
		for i := 0; i < 6; i++ {
			in <- i
		}
		close(in)
	}()
	// consume each output concurrently to avoid blocking the router
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := make([]int, len(outs))
	errs := make(chan error, len(outs))
	wg.Add(len(outs))
	for i, out := range outs {
		go func(i int, ch <-chan int) {
			defer wg.Done()
			for v := range ch {
				if v%3 != i {
					errs <- fmt.Errorf("value %d routed to wrong output %d", v, i)
					return
				}
				mu.Lock()
				counts[i]++
				mu.Unlock()
			}
		}(i, out)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected routing error: %v", err)
		}
	}

	for i := range 3 {
		if counts[i] != 2 {
			t.Fatalf("expected 2 values for output %d, got %d", i, counts[i])
		}
	}
}

func TestRoute_OutOfRangeIsDropped(t *testing.T) {
	in := make(chan int)
	outs := Route(in, func(v int) int { return -1 }, 2)

	go func() {
		in <- 1
		in <- 2
		close(in)
	}()

	for _, out := range outs {
		for range out {
			t.Fatalf("expected no values, but got some")
		}
	}
}

func TestRoute_OutputsClosedOnInputClose(t *testing.T) {
	in := make(chan int)
	outs := Route(in, func(v int) int { return 0 }, 2)
	close(in)
	// wait for each output to close (avoid blocking) using a timeout
	for _, out := range outs {
		select {
		case _, ok := <-out:
			if ok {
				t.Fatalf("expected closed output channel")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timed out waiting for output to close")
		}
	}
}
