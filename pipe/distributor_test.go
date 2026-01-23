package pipe

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestDistributor_BasicDistribute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
	})

	// Add outputs with matchers
	out1, err := dist.AddOutput(func(v int) bool { return v < 10 })
	if err != nil {
		t.Fatalf("unexpected error adding out1: %v", err)
	}
	out2, err := dist.AddOutput(func(v int) bool { return v >= 10 && v < 100 })
	if err != nil {
		t.Fatalf("unexpected error adding out2: %v", err)
	}
	out3, err := dist.AddOutput(func(v int) bool { return v >= 100 })
	if err != nil {
		t.Fatalf("unexpected error adding out3: %v", err)
	}

	input := make(chan int, 10)
	_, err = dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Start collectors before sending
	var results1, results2, results3 []int
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for v := range out1 {
			results1 = append(results1, v)
		}
	}()
	go func() {
		defer wg.Done()
		for v := range out2 {
			results2 = append(results2, v)
		}
	}()
	go func() {
		defer wg.Done()
		for v := range out3 {
			results3 = append(results3, v)
		}
	}()

	// Send values to different outputs
	input <- 1   // -> out1
	input <- 5   // -> out1
	input <- 10  // -> out2
	input <- 50  // -> out2
	input <- 100 // -> out3
	close(input)

	// Wait for all collectors to finish (output channels close when done)
	wg.Wait()

	if len(results1) != 2 || results1[0] != 1 || results1[1] != 5 {
		t.Errorf("out1: expected [1, 5], got %v", results1)
	}
	if len(results2) != 2 || results2[0] != 10 || results2[1] != 50 {
		t.Errorf("out2: expected [10, 50], got %v", results2)
	}
	if len(results3) != 1 || results3[0] != 100 {
		t.Errorf("out3: expected [100], got %v", results3)
	}
}

func TestDistributor_FirstMatchWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
	})

	// Both matchers match all values - first should win
	out1, err := dist.AddOutput(func(v int) bool { return true })
	if err != nil {
		t.Fatalf("unexpected error adding out1: %v", err)
	}
	out2, err := dist.AddOutput(func(v int) bool { return true })
	if err != nil {
		t.Fatalf("unexpected error adding out2: %v", err)
	}

	input := make(chan int, 5)
	_, err = dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Start collectors before sending
	var results1, results2 []int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for v := range out1 {
			results1 = append(results1, v)
		}
	}()
	go func() {
		defer wg.Done()
		for v := range out2 {
			results2 = append(results2, v)
		}
	}()

	input <- 1
	input <- 2
	input <- 3
	close(input)

	wg.Wait()

	// First output should get all values
	if len(results1) != 3 {
		t.Errorf("out1: expected 3 values, got %d", len(results1))
	}
	// Second output should get none
	if len(results2) != 0 {
		t.Errorf("out2: expected 0 values, got %d", len(results2))
	}
}

func TestDistributor_NilMatcherMatchesAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
	})

	// Nil matcher matches all
	out, err := dist.AddOutput(nil)
	if err != nil {
		t.Fatalf("unexpected error adding output: %v", err)
	}

	input := make(chan int, 5)
	_, err = dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Start collector before sending
	var results []int
	done := make(chan struct{})
	go func() {
		for v := range out {
			results = append(results, v)
		}
		close(done)
	}()

	input <- 1
	input <- 2
	input <- 3
	close(input)

	<-done

	if len(results) != 3 {
		t.Errorf("expected 3 values, got %d", len(results))
	}
}

func TestDistributor_ErrorHandler_NoMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var unmatched []int
	var mu sync.Mutex

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
		ErrorHandler: func(in any, err error) {
			if err == ErrNoMatchingOutput {
				mu.Lock()
				unmatched = append(unmatched, in.(int))
				mu.Unlock()
			}
		},
	})

	// Matcher that only matches even numbers
	out, err := dist.AddOutput(func(v int) bool { return v%2 == 0 })
	if err != nil {
		t.Fatalf("unexpected error adding output: %v", err)
	}

	input := make(chan int, 5)
	_, err = dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Start collector before sending
	var results []int
	done := make(chan struct{})
	go func() {
		for v := range out {
			results = append(results, v)
		}
		close(done)
	}()

	input <- 1 // unmatched
	input <- 2 // matched
	input <- 3 // unmatched
	input <- 4 // matched
	input <- 5 // unmatched
	close(input)

	<-done

	if len(results) != 2 {
		t.Errorf("matched: expected 2, got %d", len(results))
	}

	mu.Lock()
	if len(unmatched) != 3 {
		t.Errorf("unmatched: expected 3, got %d", len(unmatched))
	}
	mu.Unlock()
}

func TestDistributor_AddAfterClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer:          10,
		ShutdownTimeout: 50 * time.Millisecond,
	})

	input := make(chan int)
	_, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	// Try adding after closed - should return nil, error
	ch, err := dist.AddOutput(nil)
	if err == nil {
		t.Error("expected error when adding output after close")
	}
	if ch != nil {
		t.Error("expected nil channel when adding output after close")
	}
}

func TestDistributor_AddOutputAfterDistribute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
	})

	input := make(chan int, 10)
	_, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Add output after distribute started
	out, err := dist.AddOutput(nil)
	if err != nil {
		t.Fatalf("unexpected error adding output: %v", err)
	}

	// Start collector before sending
	var results []int
	resultsDone := make(chan struct{})
	go func() {
		for v := range out {
			results = append(results, v)
		}
		close(resultsDone)
	}()

	input <- 1
	input <- 2
	close(input)

	<-resultsDone

	if len(results) != 2 {
		t.Errorf("expected 2 values, got %d", len(results))
	}
}

func TestDistributor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer:          10,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	out, err := dist.AddOutput(nil)
	if err != nil {
		t.Fatalf("unexpected error adding output: %v", err)
	}

	input := make(chan int, 5)
	_, err = dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	input <- 1
	input <- 2
	input <- 3
	close(input)

	cancel()

	// Output should close eventually
	timeout := time.After(1 * time.Second)
	select {
	case _, ok := <-out:
		if ok {
			for range out {
			}
		}
	case <-timeout:
		t.Fatal("output channel didn't close after context cancellation")
	}
}

func TestDistributor_ShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer:          1, // Small buffer to cause blocking
		ShutdownTimeout: 100 * time.Millisecond,
	})

	out, err := dist.AddOutput(nil)
	if err != nil {
		t.Fatalf("unexpected error adding output: %v", err)
	}

	input := make(chan int, 100)
	for i := 0; i < 50; i++ {
		input <- i
	}

	_, err = dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Read one value
	<-out

	start := time.Now()
	cancel()

	// Drain output
	for range out {
	}

	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("shutdown took too long: %v", elapsed)
	}
}

func TestDistributor_MultipleDistributeReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{Buffer: 10})

	input := make(chan int)
	_, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error on first Distribute: %v", err)
	}

	_, err = dist.Distribute(ctx, input)
	if err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
}

func TestDistributor_ConcurrentAddOutputs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 100,
	})

	input := make(chan int, 100)
	_, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	var mu sync.Mutex
	outputs := []<-chan int{}
	numOutputs := 10

	// Add outputs concurrently - each matches its own id
	var addWg sync.WaitGroup
	for i := 0; i < numOutputs; i++ {
		addWg.Add(1)
		go func(id int) {
			defer addWg.Done()
			out, err := dist.AddOutput(func(v int) bool { return v == id })
			if err != nil {
				t.Errorf("unexpected error adding output: %v", err)
				return
			}
			mu.Lock()
			outputs = append(outputs, out)
			mu.Unlock()
		}(i)
	}

	addWg.Wait()

	// Start collectors before sending
	totalReceived := 0
	resultsDone := make(chan struct{})
	go func() {
		mu.Lock()
		outs := outputs
		mu.Unlock()
		var innerWg sync.WaitGroup
		for _, out := range outs {
			innerWg.Add(1)
			go func(ch <-chan int) {
				defer innerWg.Done()
				for range ch {
					mu.Lock()
					totalReceived++
					mu.Unlock()
				}
			}(out)
		}
		innerWg.Wait()
		close(resultsDone)
	}()

	// Send values matching each output
	for i := 0; i < numOutputs; i++ {
		input <- i
	}
	close(input)

	<-resultsDone

	mu.Lock()
	if totalReceived != numOutputs {
		t.Errorf("expected %d values total, got %d", numOutputs, totalReceived)
	}
	mu.Unlock()
}

func TestDistributor_DoneChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
	})

	input := make(chan int)
	done, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Done should not close while input is open
	select {
	case <-done:
		t.Error("done should not close while input is open")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	close(input)

	// Done should close after input closes
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("done should close after input closes")
	}
}

func TestDistributor_DoneClosesAfterOutputs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer: 10,
	})

	out1, err := dist.AddOutput(func(v int) bool { return v < 50 })
	if err != nil {
		t.Fatalf("unexpected error adding out1: %v", err)
	}
	out2, err := dist.AddOutput(func(v int) bool { return v >= 50 })
	if err != nil {
		t.Fatalf("unexpected error adding out2: %v", err)
	}

	input := make(chan int, 10)
	done, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Send values and close input
	input <- 1
	input <- 50
	input <- 2
	input <- 100
	close(input)

	// Wait for done to close
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("done channel did not close")
	}

	// When done closes, output channels MUST be closed.
	// Verify by checking that reads don't block and return ok=false after draining.

	// Drain out1
	for range out1 {
	}
	// Verify it's closed (this read should not block)
	select {
	case _, ok := <-out1:
		if ok {
			t.Error("out1 should be closed after draining")
		}
	default:
		t.Error("out1 read blocked - channel not closed")
	}

	// Drain out2
	for range out2 {
	}
	// Verify it's closed
	select {
	case _, ok := <-out2:
		if ok {
			t.Error("out2 should be closed after draining")
		}
	default:
		t.Error("out2 read blocked - channel not closed")
	}
}

func TestDistributor_NoGoroutineLeakWhenDistributedAndStopped(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer:          10,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	// Add several outputs
	outputs := []<-chan int{}
	for i := 0; i < 5; i++ {
		out, err := dist.AddOutput(func(v int) bool { return v == i })
		if err != nil {
			t.Fatalf("unexpected error adding output: %v", err)
		}
		outputs = append(outputs, out)
	}

	input := make(chan int, 25)
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			input <- i
		}
	}
	close(input)

	_, err := dist.Distribute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error starting distributor: %v", err)
	}

	// Consume some values
	count := 0
	for _, out := range outputs {
		for range out {
			count++
			if count >= 10 {
				cancel()
				break
			}
		}
		if count >= 10 {
			break
		}
	}

	// Drain remaining
	for _, out := range outputs {
		for range out {
		}
	}

	time.Sleep(200 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 0 {
		t.Errorf("goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}

func TestDistributor_ShutdownDrainsInput(t *testing.T) {
	// Verifies that buffered messages in input channel are reported
	// to ErrorHandler when shutdown timeout fires (no silent loss).

	var droppedCount int
	var droppedMu sync.Mutex

	dist := NewDistributor[int](DistributorConfig[int]{
		Buffer:          1, // Small buffer to cause backpressure
		ShutdownTimeout: 100 * time.Millisecond,
		ErrorHandler: func(in any, err error) {
			if err == ErrShutdownDropped {
				droppedMu.Lock()
				droppedCount++
				droppedMu.Unlock()
			}
		},
	})

	// Add output that never consumes (causes backpressure)
	out, err := dist.AddOutput(nil)
	if err != nil {
		t.Fatalf("failed to add output: %v", err)
	}

	// Create input with buffered messages
	in := make(chan int, 20)
	for i := 0; i < 20; i++ {
		in <- i
	}

	ctx, cancel := context.WithCancel(context.Background())
	done, err := dist.Distribute(ctx, in)
	if err != nil {
		t.Fatalf("failed to distribute: %v", err)
	}

	// Let some messages try to route (will block on full output)
	time.Sleep(50 * time.Millisecond)

	// Cancel to trigger shutdown
	cancel()
	close(in)

	// Wait for completion
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("distributor did not complete")
	}

	// Drain output
	for range out {
	}

	droppedMu.Lock()
	dropped := droppedCount
	droppedMu.Unlock()

	t.Logf("Dropped messages reported: %d", dropped)

	// The key assertion: messages in buffer should be reported, not silently lost
	// With 20 messages, small buffer, and short timeout, we expect drops
	if dropped == 0 {
		t.Error("expected some messages to be reported as dropped, but none were")
	}
}
