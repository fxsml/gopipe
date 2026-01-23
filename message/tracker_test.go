package message

import (
	"sync"
	"testing"
	"time"
)

func TestMessageTracker_EnterExit(t *testing.T) {
	tracker := newMessageTracker()

	tracker.enter(1)
	if tracker.inFlight() != 1 {
		t.Errorf("expected InFlight=1, got %d", tracker.inFlight())
	}

	tracker.enter(1)
	if tracker.inFlight() != 2 {
		t.Errorf("expected InFlight=2, got %d", tracker.inFlight())
	}

	tracker.exit(1)
	if tracker.inFlight() != 1 {
		t.Errorf("expected InFlight=1, got %d", tracker.inFlight())
	}

	tracker.exit(1)
	if tracker.inFlight() != 0 {
		t.Errorf("expected InFlight=0, got %d", tracker.inFlight())
	}
}

func TestMessageTracker_Drained_ClosesWhenZeroAndClosed(t *testing.T) {
	tracker := newMessageTracker()

	tracker.enter(1)
	tracker.enter(1)
	tracker.exit(1)
	tracker.exit(1)

	// Count is zero but Close() not called yet
	select {
	case <-tracker.drained():
		t.Fatal("Drained should not close before Close() is called")
	default:
		// Expected
	}

	tracker.close()

	// Now Drained should be closed
	select {
	case <-tracker.drained():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Drained should close after Close() when count is zero")
	}
}

func TestMessageTracker_Drained_NotClosedUntilClose(t *testing.T) {
	tracker := newMessageTracker()

	// Count is already zero, but Close() not called
	select {
	case <-tracker.drained():
		t.Fatal("Drained should not close before Close() is called")
	default:
		// Expected
	}
}

func TestMessageTracker_CloseWithZeroCount(t *testing.T) {
	tracker := newMessageTracker()

	// Close immediately with zero count
	tracker.close()

	select {
	case <-tracker.drained():
		// Expected - should close immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Drained should close immediately when Close() called with zero count")
	}
}

func TestMessageTracker_CloseIdempotent(t *testing.T) {
	tracker := newMessageTracker()

	tracker.close()
	tracker.close() // Should not panic
	tracker.close() // Should not panic

	select {
	case <-tracker.drained():
		// Expected
	default:
		t.Fatal("Drained should be closed")
	}
}

func TestMessageTracker_Concurrent(t *testing.T) {
	tracker := newMessageTracker()
	var wg sync.WaitGroup

	// Concurrent Enter/Exit
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.enter(1)
			time.Sleep(time.Millisecond)
			tracker.exit(1)
		}()
	}

	wg.Wait()

	if tracker.inFlight() != 0 {
		t.Errorf("expected InFlight=0 after concurrent operations, got %d", tracker.inFlight())
	}

	tracker.close()

	select {
	case <-tracker.drained():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Drained should close")
	}
}

func TestMessageTracker_DrainedClosesOnExit(t *testing.T) {
	tracker := newMessageTracker()

	tracker.enter(1)
	tracker.close()

	// Drained should not close yet (count = 1)
	select {
	case <-tracker.drained():
		t.Fatal("Drained should not close while count > 0")
	default:
		// Expected
	}

	tracker.exit(1)

	// Now Drained should close (count = 0 and closed)
	select {
	case <-tracker.drained():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Drained should close when count reaches zero after Close()")
	}
}

func TestMessageTracker_OnlyClosesOnce(t *testing.T) {
	tracker := newMessageTracker()

	// Multiple zero crossings should not panic
	tracker.enter(1)
	tracker.exit(1)
	tracker.enter(1)
	tracker.exit(1)
	tracker.enter(1)
	tracker.close()
	tracker.exit(1)

	// Should not panic - Drained is already closed
	select {
	case <-tracker.drained():
		// Expected
	default:
		t.Fatal("Drained should be closed")
	}
}

func TestMessageTracker_NegativeCount(t *testing.T) {
	// This tests the multiply case where count can temporarily go below expected
	tracker := newMessageTracker()

	tracker.enter(1)  // 1
	tracker.enter(1)  // 2
	tracker.enter(1)  // 3 (handler multiplied)
	tracker.exit(1)   // 2
	tracker.exit(1)   // 1
	tracker.exit(1)   // 0

	tracker.close()

	select {
	case <-tracker.drained():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Drained should close")
	}
}
