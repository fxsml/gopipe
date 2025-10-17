package gopipe

import (
	"context"
	"sync"
	"testing"
	"time"
)

var sinkFuncs = map[string]func(<-chan int, func(int)) <-chan struct{}{
	"Sink": Sink[int],
	"SinkPipe": func(in <-chan int, handle func(int)) <-chan struct{} {
		handlePipe := func(_ context.Context, in int) error {
			handle(in)
			return nil
		}
		return NewSinkPipe(handlePipe).Start(context.Background(), in)
	},
}

func TestSink_handleCalled(t *testing.T) {
	for name, sinkFunc := range sinkFuncs {
		t.Run(name, func(t *testing.T) {
			in := make(chan int)

			var mu sync.Mutex
			var got []int

			done := sinkFunc(in, func(v int) {
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
}

func TestSink_ExitsOnClose(t *testing.T) {
	for name, sinkFunc := range sinkFuncs {
		t.Run(name, func(t *testing.T) {
			in := make(chan int)

			senderDone := make(chan struct{})
			done := sinkFunc(in, func(v int) {
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
}

func TestSink_EmptyChannel(t *testing.T) {

	for name, sinkFunc := range sinkFuncs {
		t.Run(name, func(t *testing.T) {
			in := make(chan int)
			close(in)

			var called bool
			done := sinkFunc(in, func(v int) {
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
}
