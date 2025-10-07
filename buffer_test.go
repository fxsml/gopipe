package gopipe

import (
	"testing"
	"time"
)

func TestBuffer_Capacity(t *testing.T) {
	in := make(chan int)
	out := Buffer(in, 2)

	// send two values in a goroutine to ensure Buffer allows them without
	// blocking the sender (the buffer size is 2)
	done := make(chan struct{})
	go func() {
		in <- 1
		in <- 2
		close(in)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("send to input blocked despite buffer capacity")
	}

	// drain out
	for range out {
	}
}

func TestBuffer_FinalFlushOnClose(t *testing.T) {
	in := make(chan int, 2)
	in <- 5
	in <- 6
	close(in)

	out := Buffer(in, 2)

	var got []int
	for v := range out {
		got = append(got, v)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 values, got %v", got)
	}
}

func TestBuffer_PanicOnNegativeSize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for negative size")
		}
	}()
	_ = Buffer[int](make(chan int), -1)
}
