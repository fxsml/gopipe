package gopipeline_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipeline"
)

func TestBroadcast_AllOutputsReceiveAllValues(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)
	n := 3
	buffer := 2
	outs := gopipeline.Broadcast(ctx, n, buffer, in)

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
	ctx := context.Background()
	in := make(chan int)
	outs := gopipeline.Broadcast(ctx, 2, 1, in)
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

func TestBroadcast_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int)
	outs := gopipeline.Broadcast(ctx, 2, 1, in)
	cancel()

	time.Sleep(10 * time.Millisecond)
	for _, out := range outs {
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
}
