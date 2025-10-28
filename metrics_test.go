package gopipe

import (
	"context"
	"testing"
	"time"
)

func TestUseMetrics_Basic(t *testing.T) {
	var got []*Metrics
	collector := func(m *Metrics) {
		got = append(got, m)
	}

	proc := NewProcessor(
		func(ctx context.Context, in int) ([]int, error) {
			time.Sleep(10 * time.Millisecond)
			return []int{in * 2}, nil
		},
		nil,
	)
	mw := useMetrics[int, int](collector)
	procWithMetrics := mw(proc)

	_, _ = procWithMetrics.Process(context.Background(), 5)

	if len(got) != 1 {
		t.Fatalf("expected 1 metrics, got %d", len(got))
	}
	if got[0].OutputCount != 1 || got[0].InputCount != 1 {
		t.Errorf("unexpected metrics: %+v", got[0])
	}
	if got[0].Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", got[0].Duration)
	}
}

func TestUseMetrics_WithStartProcessor(t *testing.T) {
	var got []*Metrics
	collector := func(m *Metrics) {
		got = append(got, m)
	}

	proc := NewProcessor(
		func(ctx context.Context, in int) ([]int, error) {
			return []int{in + 1}, nil
		},
		nil,
	)
	mw := useMetrics[int, int](collector)
	procWithMetrics := mw(proc)

	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	out := StartProcessor(context.Background(), in, procWithMetrics)
	for range out {
		// drain output
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(got))
	}
	for i, m := range got {
		if m.OutputCount != 1 || m.InputCount != 1 {
			t.Errorf("metrics[%d] unexpected: %+v", i, m)
		}
	}
}

func TestNewMetricsDistributor(t *testing.T) {
	var got1, got2 []*Metrics

	collector1 := func(m *Metrics) { got1 = append(got1, m) }
	collector2 := func(m *Metrics) { got2 = append(got2, m) }

	distributor := newMetricsDistributor(collector1, collector2)

	// Send some metrics
	for i := range 3 {
		distributor(&Metrics{InputCount: 1, OutputCount: i})
	}

	// Both collectors should have received all metrics
	if len(got1) != 3 || len(got2) != 3 {
		t.Errorf("expected 3 metrics in each collector, got %d and %d", len(got1), len(got2))
	}
	for i := range 3 {
		if got1[i].OutputCount != i || got2[i].OutputCount != i {
			t.Errorf("unexpected OutputCount at index %d: got1=%d, got2=%d", i, got1[i].OutputCount, got2[i].OutputCount)
		}
	}
}
