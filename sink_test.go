package gopipe

import (
	"sync"
	"testing"
	"time"
)

func TestSink_handleCalled(t *testing.T) {
	in := make(chan int)

	var mu sync.Mutex
	var got []int

	done := Sink(in, func(v int) {
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	})

	go func() {
		in <- 1
		in <- 2
		close(in)
	}()

	// Wait for done channel to close, indicating all values were processed
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel did not close after input was closed and processed")
	}

	mu.Lock()
	if len(got) != 2 {
		t.Fatalf("expected handle to be called twice, got %d", len(got))
	}
	mu.Unlock()
}

func TestSink_ExitsOnClose(t *testing.T) {
	in := make(chan int)

	senderDone := make(chan struct{})
	done := Sink(in, func(v int) {
		// no-op
	})

	go func() {
		in <- 1
		close(in)
		close(senderDone)
	}()

	// Verify sender completes
	select {
	case <-senderDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sink did not allow sender to finish")
	}

	// Verify done channel closes after processing is complete
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("done channel did not close after input was closed and processed")
	}
}

func TestSink_EmptyChannel(t *testing.T) {
	in := make(chan int)
	close(in)

	var called bool
	done := Sink(in, func(v int) {
		called = true
	})

	// Verify done channel closes immediately when an empty channel is closed
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel did not close after empty input was closed")
	}

	if called {
		t.Fatal("handler was called even though input channel was empty")
	}
}
