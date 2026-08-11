package pipe

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// testMetrics is a thread-safe Metrics implementation for use in tests.
type testMetrics struct {
	mu         sync.Mutex
	processing []ProcessingInfo
	waits      []WaitInfo
}

func (m *testMetrics) RecordProcessing(_ context.Context, info ProcessingInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processing = append(m.processing, info)
}

func (m *testMetrics) RecordWait(_ context.Context, info WaitInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waits = append(m.waits, info)
}

func (m *testMetrics) receives() []WaitInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []WaitInfo
	for _, w := range m.waits {
		if w.Operation == "receive" {
			out = append(out, w)
		}
	}
	return out
}

func (m *testMetrics) sends() []WaitInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []WaitInfo
	for _, w := range m.waits {
		if w.Operation == "send" {
			out = append(out, w)
		}
	}
	return out
}

// --- startProcessing ---

func TestProcessing_Metrics_NilSafe(t *testing.T) {
	t.Parallel()
	in := make(chan int, 1)
	in <- 1
	close(in)
	out := startProcessing(context.Background(), in, func(_ context.Context, v int) ([]int, error) {
		return []int{v}, nil
	}, Config{Metrics: nil})
	for range out {
	}
}

func TestProcessing_Metrics_RecordProcessing_Success(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	in := make(chan int, 2)
	in <- 7
	in <- 13
	close(in)

	out := startProcessing(context.Background(), in, func(_ context.Context, v int) ([]int, error) {
		return []int{v, v}, nil // two outputs each
	}, Config{
		Concurrency: 1,
		Metrics:     m,
		Labels:      map[string]string{"stage": "test"},
	})
	for range out {
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.processing) != 2 {
		t.Fatalf("want 2 RecordProcessing calls, got %d", len(m.processing))
	}
	for i, info := range m.processing {
		if info.Labels["stage"] != "test" {
			t.Errorf("call %d: want label stage=test, got %v", i, info.Labels)
		}
		if info.Error != nil {
			t.Errorf("call %d: want nil error, got %v", i, info.Error)
		}
		if info.OutputCount != 2 {
			t.Errorf("call %d: want OutputCount=2, got %d", i, info.OutputCount)
		}
		if info.Duration <= 0 {
			t.Errorf("call %d: want positive duration, got %v", i, info.Duration)
		}
	}
}

func TestProcessing_Metrics_RecordProcessing_Error(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	boom := errors.New("boom")
	in := make(chan int, 1)
	in <- 1
	close(in)

	out := startProcessing(context.Background(), in, func(_ context.Context, _ int) ([]int, error) {
		return nil, boom
	}, Config{
		Metrics:      m,
		ErrorHandler: func(any, error) {},
	})
	for range out {
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.processing) != 1 {
		t.Fatalf("want 1 RecordProcessing call, got %d", len(m.processing))
	}
	if !errors.Is(m.processing[0].Error, boom) {
		t.Errorf("want error %v, got %v", boom, m.processing[0].Error)
	}
	if m.processing[0].OutputCount != 0 {
		t.Errorf("want OutputCount=0 on error, got %d", m.processing[0].OutputCount)
	}
}

func TestProcessing_Metrics_RecordWait_ReceiveAndSend(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	out := startProcessing(context.Background(), in, func(_ context.Context, v int) ([]int, error) {
		return []int{v}, nil
	}, Config{Concurrency: 1, Metrics: m, BufferSize: 10})
	for range out {
	}

	if got := len(m.receives()); got != 3 {
		t.Errorf("want 3 receive-wait events, got %d", got)
	}
	if got := len(m.sends()); got != 3 {
		t.Errorf("want 3 send-wait events, got %d", got)
	}
}

// TestProcessing_Metrics_NoReceiveWaitOnChannelClose verifies that closing the
// input channel does not generate a spurious receive-wait event (bug fix: the
// ok==false path must exit before calling RecordWait).
func TestProcessing_Metrics_NoReceiveWaitOnChannelClose(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	const n = 3
	in := make(chan int, n)
	for i := range n {
		in <- i
	}
	close(in)

	out := startProcessing(context.Background(), in, func(_ context.Context, v int) ([]int, error) {
		return []int{v}, nil
	}, Config{Concurrency: 1, Metrics: m, BufferSize: n})
	for range out {
	}

	if got := len(m.receives()); got != n {
		t.Errorf("want exactly %d receive-wait events (no extra for channel close), got %d", n, got)
	}
}

func TestProcessPipe_Stats(t *testing.T) {
	t.Parallel()
	p := NewProcessPipe(func(_ context.Context, v int) ([]int, error) {
		return []int{v}, nil
	}, Config{BufferSize: 8})

	// Stats before start returns zero value.
	if s := p.Stats(); s.Depth != 0 || s.Capacity != 0 {
		t.Errorf("pre-start: want zero BufferStats, got %+v", s)
	}

	in := make(chan int, 1)
	in <- 42
	close(in)
	out, err := p.Pipe(context.Background(), in)
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	for range out {
	}

	s := p.Stats()
	if s.Capacity != 8 {
		t.Errorf("want Capacity=8, got %d", s.Capacity)
	}
}

func TestProcessing_Metrics_Labels(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	in := make(chan int, 1)
	in <- 1
	close(in)
	labels := map[string]string{"pipeline": "orders", "stage": "router"}

	out := startProcessing(context.Background(), in, func(_ context.Context, v int) ([]int, error) {
		return []int{v}, nil
	}, Config{Metrics: m, Labels: labels, BufferSize: 1})
	for range out {
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, info := range m.processing {
		if info.Labels["pipeline"] != "orders" {
			t.Errorf("processing: want pipeline=orders, got %v", info.Labels)
		}
	}
	for _, w := range m.waits {
		if w.Labels["stage"] != "router" {
			t.Errorf("wait: want stage=router, got %v", w.Labels)
		}
	}
}

// --- Merger ---

func TestMerger_Metrics_RecordWait(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{
		Buffer:  4,
		Metrics: m,
		Labels:  map[string]string{"stage": "merge"},
	})

	ch := make(chan int, 2)
	ch <- 10
	ch <- 20
	close(ch)

	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("AddInput: %v", err)
	}

	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Cancel once input is fully consumed so the output channel closes.
	go func() { <-done; cancel() }()

	var got []int
	for v := range out {
		got = append(got, v)
	}
	if len(got) != 2 {
		t.Errorf("want 2 merged items, got %d", len(got))
	}

	if got := len(m.sends()); got != 2 {
		t.Errorf("want 2 send-wait events, got %d", got)
	}
	for _, w := range m.sends() {
		if w.Labels["stage"] != "merge" {
			t.Errorf("send-wait: want stage=merge, got %v", w.Labels)
		}
	}
}

func TestMerger_Stats(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{Buffer: 4})

	ch := make(chan int, 1)
	ch <- 99
	close(ch)

	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("AddInput: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	go func() { <-done; cancel() }()
	for range out {
	}

	s := merger.Stats()
	if s.Capacity != 4 {
		t.Errorf("want Capacity=4, got %d", s.Capacity)
	}
}

func TestMerger_Metrics_NilSafe(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{Buffer: 2, Metrics: nil})
	ch := make(chan int, 1)
	ch <- 1
	close(ch)

	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("AddInput: %v", err)
	}
	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	go func() { <-done; cancel() }()
	for range out {
	}
}

// TestMerger_Metrics_PostMergeAddInput verifies that metrics fire for inputs
// added after Merge() is already running.
func TestMerger_Metrics_PostMergeAddInput(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	ctx, cancel := context.WithCancel(context.Background())

	merger := NewMerger[int](MergerConfig{Buffer: 4, Metrics: m})

	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	ch := make(chan int, 2)
	ch <- 5
	ch <- 6
	close(ch)

	done, err := merger.AddInput(ch)
	if err != nil {
		t.Fatalf("AddInput after Merge: %v", err)
	}

	go func() { <-done; cancel() }()

	var got []int
	for v := range out {
		got = append(got, v)
	}
	if len(got) != 2 {
		t.Errorf("want 2 items, got %d", len(got))
	}
	if gs := len(m.sends()); gs != 2 {
		t.Errorf("want 2 send-wait events, got %d", gs)
	}
}

// --- Distributor ---

func TestDistributor_Metrics_RecordWait(t *testing.T) {
	t.Parallel()
	m := &testMetrics{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor(DistributorConfig[int]{
		Buffer:  4,
		Metrics: m,
		Labels:  map[string]string{"stage": "dist"},
	})

	out, err := dist.AddOutput(nil) // match all
	if err != nil {
		t.Fatalf("AddOutput: %v", err)
	}

	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	done, err := dist.Distribute(ctx, in)
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}

	var got []int
	for v := range out {
		got = append(got, v)
	}
	<-done

	if len(got) != 3 {
		t.Errorf("want 3 items, got %d", len(got))
	}
	if gs := len(m.sends()); gs != 3 {
		t.Errorf("want 3 send-wait events, got %d", gs)
	}
	for _, w := range m.sends() {
		if w.Labels["stage"] != "dist" {
			t.Errorf("send-wait: want stage=dist, got %v", w.Labels)
		}
	}
}

func TestDistributor_Stats(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor(DistributorConfig[int]{Buffer: 4})
	_, err := dist.AddOutput(nil) // first output
	if err != nil {
		t.Fatalf("AddOutput: %v", err)
	}
	_, err = dist.AddOutput(nil) // second output (unreachable, first matches all)
	if err != nil {
		t.Fatalf("AddOutput: %v", err)
	}

	in := make(chan int, 1)
	close(in)
	_, err = dist.Distribute(ctx, in)
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}

	// Stats() aggregates across outputs.
	s := dist.Stats()
	if s.Capacity != 4 {
		t.Errorf("Stats: want Capacity=4, got %d", s.Capacity)
	}

	// OutputStats() returns one entry per registered output.
	os := dist.OutputStats()
	if len(os) != 2 {
		t.Fatalf("OutputStats: want 2 entries (one per output), got %d", len(os))
	}
	for i, s := range os {
		if s.Capacity != 4 {
			t.Errorf("OutputStats[%d]: want Capacity=4, got %d", i, s.Capacity)
		}
	}
}

func TestDistributor_Metrics_NilSafe(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dist := NewDistributor(DistributorConfig[int]{Buffer: 2, Metrics: nil})
	out, _ := dist.AddOutput(nil)

	in := make(chan int, 1)
	in <- 42
	close(in)

	done, err := dist.Distribute(ctx, in)
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}
	for range out {
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("distributor did not finish")
	}
}
