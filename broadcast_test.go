package gopipe_test

import (
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe"
)

func TestBroadcast_AllOutputsReceiveAllValues(t *testing.T) {
	in := make(chan int)
	n := 3
	outs := gopipe.Broadcast(in, n)

	go func() {
		for i := 1; i <= 5; i++ {
			in <- i
		}
		close(in)
	}()

	var wg sync.WaitGroup
	for i, out := range outs {
		wg.Add(1)
		go func(idx int, ch <-chan int) {
			defer wg.Done()
			var got []int
			for v := range ch {
				got = append(got, v)
			}
			if len(got) != 5 {
				t.Errorf("output %d: expected 5 values, got %d", idx, len(got))
			}
			for j, v := range got {
				if v != j+1 {
					t.Errorf("output %d: expected %d at pos %d, got %d", idx, j+1, j, v)
				}
			}
		}(i, out)
	}
	wg.Wait()
}

func TestBroadcast_ChannelsClosedOnInputClose(t *testing.T) {
	in := make(chan int)
	outs := gopipe.Broadcast(in, 5)
	close(in)

	time.Sleep(10 * time.Millisecond)
	for i, out := range outs {
		select {
		case _, ok := <-out:
			if ok {
				t.Errorf("output %d: expected closed channel", i)
			}
		default:
		}
	}
}

func TestBroadcast_GlobalBlocking(t *testing.T) {
	in := make(chan int)
	outs := gopipe.Broadcast(in, 3)

	// make one consumer slow (never read) to observe that broadcaster blocks
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// read only from the first output
		for v := range outs[0] {
			_ = v
		}
	}()

	// send two values; the second send should block because the broadcaster
	// can't progress past the slow consumer. The sender goroutine will close
	// in and signal done when both sends complete.
	done := make(chan struct{})
	go func() {
		in <- 1
		in <- 2
		close(in)
		close(done)
	}()

	// after a short delay the sender should still be blocked on the second send
	select {
	case <-done:
		t.Fatalf("expected broadcaster to block when some consumers are slow")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked
	}

	// now start readers for the remaining outputs to allow the broadcaster to
	// finish and the sender to complete
	go func() {
		for v := range outs[1] {
			_ = v
		}
	}()
	go func() {
		for v := range outs[2] {
			_ = v
		}
	}()

	// wait for the sender to finish
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for sender to finish after unblocking consumers")
	}

	// cleanup: wait for the first reader to finish after channels are closed
	wg.Wait()
}

func TestBroadcast_PanicsOnNegativeN(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for negative n")
		}
	}()
	_ = gopipe.Broadcast[int](make(chan int), -1)
}
