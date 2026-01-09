package pipe

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// assertDoneClosed asserts that the done channel is closed within the timeout.
// This helper ensures that Merger properly signals completion of input channel processing.
func assertDoneClosed(t *testing.T, done <-chan struct{}, timeout time.Duration, channelName string) {
	t.Helper()
	select {
	case <-done:
		// Success - done channel is closed
	case <-time.After(timeout):
		t.Errorf("done channel for %s was not closed within %v", channelName, timeout)
	}
}

func TestMerger_BasicMerge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 1 * time.Second, // Allow time for graceful drain
	})

	ch1 := make(chan int, 2)
	ch2 := make(chan int, 2)
	ch3 := make(chan int, 1)

	done1, err := merger.AddInput(ch1)
	if err != nil {
		t.Fatalf("unexpected error adding ch1: %v", err)
	}
	done2, err := merger.AddInput(ch2)
	if err != nil {
		t.Fatalf("unexpected error adding ch2: %v", err)
	}
	done3, err := merger.AddInput(ch3)
	if err != nil {
		t.Fatalf("unexpected error adding ch3: %v", err)
	}

	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Send values from different channels
	go func() {
		ch1 <- 1
		ch1 <- 2
		close(ch1)
	}()
	go func() {
		ch2 <- 10
		ch2 <- 20
		close(ch2)
	}()
	go func() {
		ch3 <- 100
		close(ch3)
	}()

	// Wait for all inputs to close, then cancel
	assertDoneClosed(t, done1, 1*time.Second, "ch1")
	assertDoneClosed(t, done2, 1*time.Second, "ch2")
	assertDoneClosed(t, done3, 1*time.Second, "ch3")

	cancel()

	// Collect results
	results := make(map[int]bool)
	for v := range out {
		results[v] = true
	}

	expected := []int{1, 2, 10, 20, 100}
	if len(results) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(results))
	}

	for _, v := range expected {
		if !results[v] {
			t.Errorf("missing value %d", v)
		}
	}
}

func TestMerger_AddAfterClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 1,
	})
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	ch1 := make(chan int)
	done1, err := merger.AddInput(ch1)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}

	// Close context to trigger shutdown
	cancel()

	// Wait a bit for shutdown to complete
	time.Sleep(50 * time.Millisecond)

	// Try adding after closed - should return nil, error
	ch2 := make(chan int, 1)
	ch2 <- 99
	close(ch2)

	done2, err2 := merger.AddInput(ch2)
	if err2 == nil {
		t.Error("expected error when adding channel after close")
	}
	if done2 != nil {
		t.Error("expected nil done channel when adding channel after close")
	}

	// Drain output
	for o := range out {
		if o == 99 {
			t.Errorf("received value from channel added after close: %d", o)
		}
	}

	// Assert done channel is closed (ch1 was added before shutdown)
	close(ch1) // Close ch1 so done1 can close
	assertDoneClosed(t, done1, 1*time.Second, "ch1")
}

func TestMerger_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 1,
	})

	ch := make(chan int, 5)
	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Send some values
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch) // Close channel before cancel

	// Cancel context
	cancel()

	// Output should close eventually
	timeout := time.After(1 * time.Second)
	select {
	case _, ok := <-out:
		if ok {
			// Drain remaining
			for range out {
			}
		}
	case <-timeout:
		t.Fatal("output channel didn't close after context cancellation")
	}

	// Assert done channel is closed
	assertDoneClosed(t, done, 1*time.Second, "ch")
}

func TestMerger_ShutdownDuration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	// Channel that won't close naturally
	ch := make(chan int, 100)
	for i := 0; i < 50; i++ {
		ch <- i
	}

	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Read a few values
	<-out
	<-out

	start := time.Now()
	cancel()

	// Drain output
	count := 0
	for range out {
		count++
	}

	elapsed := time.Since(start)

	// Should shutdown within ShutdownDuration
	if elapsed > 200*time.Millisecond {
		t.Errorf("shutdown took too long: %v", elapsed)
	}

	// Should have received some values during shutdown
	if count == 0 {
		t.Error("expected to receive some values during shutdown")
	}

	// Assert done channel is closed (should be closed after shutdown timeout)
	assertDoneClosed(t, done, 500*time.Millisecond, "ch")
}

func TestMerger_ForcedShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 50 * time.Millisecond, // Force after 50ms
	})

	ch := make(chan int, 10)
	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Send some values before cancel (don't close ch)
	ch <- 1
	ch <- 2

	// Cancel - should force shutdown after timeout since ch isn't closed
	cancel()

	// Output should close after timeout
	timeout := time.After(500 * time.Millisecond)
	select {
	case <-out:
		// Drain any values that made it through
		for range out {
		}
	case <-timeout:
		t.Fatal("output channel didn't close after forced shutdown")
	}

	// Assert done channel is closed
	assertDoneClosed(t, done, 1*time.Second, "ch")
}

func TestMerger_NaturalCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 0, // Wait for natural completion
	})

	ch := make(chan int, 10)
	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Send values and close input
	ch <- 1
	ch <- 2
	close(ch)

	// Cancel context
	cancel()

	// Should complete naturally since input is closed
	results := []int{}
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 values, got %d", len(results))
	}

	// Assert done channel is closed
	assertDoneClosed(t, done, 1*time.Second, "ch")
}

func TestMerger_ConcurrentAdds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger := NewMerger[int](MergerConfig{
		Buffer:          100,
		ShutdownTimeout: 1 * time.Second, // Allow time for processing
	})
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	var wg sync.WaitGroup
	var doneMu sync.Mutex
	doneChs := []<-chan struct{}{}
	numChannels := 10

	// Add channels concurrently
	for i := range numChannels {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ch := make(chan int, 5)
			done, err := merger.AddInput(ch)
			if err != nil {
				t.Errorf("unexpected error adding channel: %v", err)
				return
			}
			doneMu.Lock()
			doneChs = append(doneChs, done)
			doneMu.Unlock()
			for j := range 5 {
				ch <- id*10 + j
			}
			close(ch)
		}(i)
	}

	// Collect results
	done := make(chan struct{})
	results := make(map[int]bool)
	go func() {
		for v := range out {
			results[v] = true
		}
		close(done)
	}()

	// Wait for all adds to complete
	wg.Wait()
	cancel()
	<-done

	// Should have numChannels * 5 unique values
	expected := numChannels * 5
	if len(results) != expected {
		t.Errorf("expected %d unique values, got %d", expected, len(results))
	}

	// Assert all done channels are closed
	for i, done := range doneChs {
		assertDoneClosed(t, done, 1*time.Second, "concurrent channel")
		if t.Failed() {
			t.Logf("Failed on done channel %d", i)
			break
		}
	}
}

func TestMerger_EmptyChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 0,
	})

	ch := make(chan int)
	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Close empty channel
	close(ch)
	cancel()

	// Output should close with no values
	count := 0
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("expected no values from empty channel, got %d", count)
	}

	// Assert done channel is closed even for empty channel
	assertDoneClosed(t, done, 1*time.Second, "ch")
}

func TestMerger_MultipleInputsOneCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 0,
	})

	ch1 := make(chan int)
	ch2 := make(chan int)
	ch3 := make(chan int)

	done1, err := merger.AddInput(ch1)
	if err != nil {
		t.Fatalf("unexpected error adding ch1: %v", err)
	}
	done2, err := merger.AddInput(ch2)
	if err != nil {
		t.Fatalf("unexpected error adding ch2: %v", err)
	}
	done3, err := merger.AddInput(ch3)
	if err != nil {
		t.Fatalf("unexpected error adding ch3: %v", err)
	}

	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// ch1 sends and closes immediately
	go func() {
		ch1 <- 1
		close(ch1)
	}()

	// ch2 sends continuously
	go func() {
		for i := 10; i < 15; i++ {
			ch2 <- i
			time.Sleep(10 * time.Millisecond)
		}
		close(ch2)
	}()

	// ch3 sends and closes
	go func() {
		ch3 <- 100
		ch3 <- 101
		close(ch3)
	}()

	// Collect all results in background
	done := make(chan struct{})
	results := []int{}
	go func() {
		for v := range out {
			results = append(results, v)
		}
		close(done)
	}()

	// Wait a bit for all channels to be processed
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	// Should receive all values from all channels
	if len(results) < 8 { // 1 + 5 + 2
		t.Errorf("expected at least 8 values, got %d", len(results))
	}

	// Assert all done channels are closed
	assertDoneClosed(t, done1, 1*time.Second, "ch1")
	assertDoneClosed(t, done2, 1*time.Second, "ch2")
	assertDoneClosed(t, done3, 1*time.Second, "ch3")
}

func TestMerger_BufferFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:          2, // Small buffer
		ShutdownTimeout: 100 * time.Millisecond,
	})

	ch := make(chan int, 5)
	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("unexpected error adding channel: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Fill buffer and channel
	ch <- 1
	ch <- 2
	ch <- 3
	ch <- 4
	ch <- 5

	// Read one value to unblock
	v := <-out
	if v < 1 || v > 5 {
		t.Errorf("unexpected value: %d", v)
	}

	close(ch)
	cancel()

	// Should be able to drain remaining
	count := 0
	for range out {
		count++
	}

	if count < 4 {
		t.Errorf("expected at least 4 more values, got %d", count)
	}

	// Assert done channel is closed
	assertDoneClosed(t, done, 1*time.Second, "ch")
}

func TestMerger_MultipleMergeReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger := NewMerger[int](MergerConfig{Buffer: 10})

	// First Merge() should work
	_, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error on first Merge: %v", err)
	}

	// Second Merge() should return ErrAlreadyStarted
	_, err = merger.Merge(ctx)
	if err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
}

func TestMerger_NoGoroutineLeakWhenMergedAndStopped(t *testing.T) {
	// Get initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	// Add several channels
	doneChs := []<-chan struct{}{}
	for range 5 {
		ch := make(chan int, 10)
		done, err := merger.AddInput(ch)
		if err != nil {
			t.Fatalf("unexpected error adding channel: %v", err)
		}
		doneChs = append(doneChs, done)
		// Send some data and close
		go func() {
			for j := 0; j < 5; j++ {
				ch <- j
			}
			close(ch)
		}()
	}

	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting merger: %v", err)
	}

	// Consume some values
	count := 0
	for range out {
		count++
		if count >= 10 {
			// Cancel early
			cancel()
			break
		}
	}

	// Drain remaining
	for range out {
	}

	// Give goroutines time to clean up
	time.Sleep(200 * time.Millisecond)

	// Check for goroutine leaks
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}

	// Assert all done channels are closed
	for i, done := range doneChs {
		assertDoneClosed(t, done, 1*time.Second, "channel")
		if t.Failed() {
			t.Logf("Failed on done channel %d", i)
			break
		}
	}
}

func TestMerger_NoGoroutineLeakWhenNeverMerged(t *testing.T) {
	// Get initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	merger := NewMerger[int](MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	// Add several channels without starting
	for range 5 {
		ch := make(chan int, 10)
		if _, err := merger.AddInput(ch); err != nil {
			t.Fatalf("unexpected error adding channel: %v", err)
		}
	}

	// Give goroutines time to potentially leak
	time.Sleep(100 * time.Millisecond)

	// Check for goroutine leaks
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}
