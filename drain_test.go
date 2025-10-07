package gopipe

import (
	"testing"
	"time"
)

func TestDrain_NonBlockingProducer(t *testing.T) {
	in := make(chan int, 1)
	done := Drain(in)

	// with buffer 1 and Drain running, sending two values should not block
	in <- 1
	select {
	case in <- 2:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("send blocked even though Drain should consume")
	}

	close(in)

	// Verify done channel closes after input is drained
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel did not close after input was closed and drained")
	}
}

func TestDrain_ExitsOnClose(t *testing.T) {
	in := make(chan int)
	doneCh := Drain(in)

	// start a goroutine to send then close
	senderDone := make(chan struct{})
	go func() {
		in <- 5
		close(in)
		close(senderDone)
	}()

	select {
	case <-senderDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("drain goroutine did not allow sender to finish")
	}

	// Verify doneCh closes after input is drained
	select {
	case <-doneCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel did not close after input was closed and drained")
	}
}
func TestDrain_Basic(t *testing.T) {
	in := make(chan int, 2)
	in <- 1
	in <- 2
	close(in)

	done := Drain(in)

	// Verify done channel closes after input is drained
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel did not close after input was closed and drained")
	}
}

func TestDrain_EmptyChannel(t *testing.T) {
	in := make(chan int)
	close(in)

	done := Drain(in)

	// Verify done channel closes immediately when an empty channel is closed
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel did not close after empty input was closed")
	}
}
