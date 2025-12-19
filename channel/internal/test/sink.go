package test

import (
	"sync"
	"testing"
	"time"
)

type SinkFunc func(<-chan int, func(int)) <-chan struct{}

func RunSink_handleCalled(t *testing.T, f SinkFunc) {
	t.Run("sink handle called", func(t *testing.T) {
		in := make(chan int)

		var mu sync.Mutex
		var got []int

		done := f(in, func(v int) {
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
	})
}

func RunSink_ExitsOnClose(t *testing.T, f SinkFunc) {
	t.Run("sink exits on close", func(t *testing.T) {
		in := make(chan int)

		senderDone := make(chan struct{})
		done := f(in, func(v int) {
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
	})
}

func RunSink_EmptyChannel(t *testing.T, f SinkFunc) {
	t.Run("sink handles empty channel", func(t *testing.T) {
		in := make(chan int)
		close(in)

		var called bool
		done := f(in, func(v int) {
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
	})
}
