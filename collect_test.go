package gopipe

import (
	"testing"
	"time"
)

func TestCollect_BySize(t *testing.T) {
	in := make(chan int)
	out := Collect(in, 3, time.Hour)

	go func() {
		in <- 1
		in <- 2
		in <- 3
		// keep the sender goroutine alive until we've read the batch
		close(in)
	}()

	select {
	case batch, ok := <-out:
		if !ok {
			t.Fatalf("output closed unexpectedly")
		}
		if len(batch) != 3 {
			t.Fatalf("expected batch of size 3, got %d", len(batch))
		}
		if batch[0] != 1 || batch[1] != 2 || batch[2] != 3 {
			t.Fatalf("unexpected batch contents: %v", batch)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for size-based batch")
	}

	// after close, out should be closed as well (no more batches)
	select {
	case _, ok := <-out:
		if ok {
			t.Fatalf("expected out to be closed after final batch")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for output close")
	}
}

func TestCollect_ByTime(t *testing.T) {
	in := make(chan int)
	out := Collect(in, 10, 50*time.Millisecond)

	go func() {
		in <- 7
		in <- 8
		// leave channel open so Collect flushes on the ticker
	}()

	select {
	case batch, ok := <-out:
		if !ok {
			t.Fatalf("output closed unexpectedly")
		}
		if len(batch) != 2 || batch[0] != 7 || batch[1] != 8 {
			t.Fatalf("unexpected batch from timer: %v", batch)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for time-based batch")
	}

	// close input to allow Collect to finish and close out
	close(in)
	select {
	case _, ok := <-out:
		if ok {
			t.Fatalf("expected out to be closed after final batch")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for output close")
	}
}

func TestCollect_FinalFlushOnClose(t *testing.T) {
	in := make(chan int, 2)
	in <- 4
	in <- 5
	close(in)

	out := Collect(in, 10, time.Hour)

	select {
	case batch, ok := <-out:
		if !ok {
			t.Fatalf("output closed unexpectedly")
		}
		if len(batch) != 2 || batch[0] != 4 || batch[1] != 5 {
			t.Fatalf("unexpected final batch: %v", batch)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for final batch on close")
	}

	select {
	case _, ok := <-out:
		if ok {
			t.Fatalf("expected out to be closed after final batch")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for output close")
	}
}

func TestCollect_PanicsOnInvalidArgs(t *testing.T) {
	t.Run("negative maxSize", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic for negative maxSize")
			}
		}()
		_ = Collect[int](make(chan int), -1, time.Second)
	})

	t.Run("non-positive duration", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic for non-positive duration")
			}
		}()
		_ = Collect(make(chan int), 1, 0)
	})
}
