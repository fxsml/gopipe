package otel_test

// Benchmarks for PipeMetrics hot-path allocation behaviour.
//
// A noop meter is used so that allocations inside the OTel SDK do not
// obscure allocations originating in PipeMetrics itself. Each benchmark
// therefore counts only the heap escapes caused by PipeMetrics code.
//
// Run with:
//
//	go test -bench=BenchmarkRecord -benchmem ./message/otel/
//
// Two allocation issues are under test:
//
//  1. labelsToAttrs: called on every RecordProcessing / RecordWait call;
//     allocates a fresh []attribute.KeyValue even when the label set is
//     identical to the previous call (e.g. all messages of the same CE type).
//
//  2. totalAttrs in RecordProcessing: unconditionally allocates a new
//     []attribute.KeyValue (len(attrs)+1) to append error.type="" on
//     the success path, where no allocation is necessary.

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	gopelotel "github.com/fxsml/gopipe/message/otel"
	"github.com/fxsml/gopipe/pipe"
)

func newNoopPipeMetrics(b *testing.B) *gopelotel.PipeMetrics {
	b.Helper()
	pm, err := gopelotel.NewPipeMetrics(noop.NewMeterProvider().Meter("bench"))
	if err != nil {
		b.Fatalf("NewPipeMetrics: %v", err)
	}
	return pm
}

// ── RecordProcessing ─────────────────────────────────────────────────────────

// BenchmarkRecordProcessing_NoLabels is the zero-overhead baseline: no labels,
// success path. Only the totalAttrs allocation (issue B) should be visible.
func BenchmarkRecordProcessing_NoLabels(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.ProcessingInfo{Duration: time.Millisecond, OutputCount: 1}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordProcessing(ctx, info)
	}
}

// BenchmarkRecordProcessing_StaticLabels has a fixed label set.
// labelsToAttrs (issue A) and totalAttrs (issue B) both allocate on every call.
func BenchmarkRecordProcessing_StaticLabels(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.ProcessingInfo{
		Labels:      map[string]string{"pipeline": "orders", "stage": "router"},
		Duration:    time.Millisecond,
		OutputCount: 1,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordProcessing(ctx, info)
	}
}

// BenchmarkRecordProcessing_MergedLabels simulates the default message-package
// behaviour: static labels merged with a cloudevents.type dimension.
// This is the common production case.
func BenchmarkRecordProcessing_MergedLabels(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.ProcessingInfo{
		Labels: map[string]string{
			"pipeline":         "orders",
			"stage":            "router",
			"cloudevents.type": "process.order",
		},
		Duration:    time.Millisecond,
		OutputCount: 1,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordProcessing(ctx, info)
	}
}

// BenchmarkRecordProcessing_Error benchmarks the error path.
// fmt.Sprintf for the error type adds one allocation vs the success path.
func BenchmarkRecordProcessing_Error(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.ProcessingInfo{
		Labels:   map[string]string{"pipeline": "orders", "stage": "router"},
		Duration: time.Millisecond,
		Error:    errBench,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordProcessing(ctx, info)
	}
}

// ── RecordWait ────────────────────────────────────────────────────────────────

// BenchmarkRecordWait_NoLabels is the zero-overhead baseline for RecordWait.
func BenchmarkRecordWait_NoLabels(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.WaitInfo{Operation: pipe.WaitOpReceive, Duration: time.Millisecond}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordWait(ctx, info)
	}
}

// BenchmarkRecordWait_StaticLabels mirrors the receive-wait call in
// processing.go, where cfg.Labels (static) is passed on every dequeue.
// labelsToAttrs (issue A) allocates on every call.
func BenchmarkRecordWait_StaticLabels(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.WaitInfo{
		Labels:    map[string]string{"pipeline": "orders", "stage": "router"},
		Operation: pipe.WaitOpReceive,
		Duration:  time.Millisecond,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordWait(ctx, info)
	}
}

// BenchmarkRecordWait_MergedLabels mirrors send-wait calls after mergeLabels.
func BenchmarkRecordWait_MergedLabels(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	info := pipe.WaitInfo{
		Labels: map[string]string{
			"pipeline":         "orders",
			"stage":            "router",
			"cloudevents.type": "process.order",
		},
		Operation: pipe.WaitOpSend,
		Duration:  time.Millisecond,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pm.RecordWait(ctx, info)
	}
}

// BenchmarkRecordProcessing_HighCardinality simulates a LabelFunc that emits
// a high-cardinality dimension (e.g. order ID or user ID). This is relevant
// for any caching strategy: a sync.Map cache keyed by the label set would
// grow without bound under high cardinality, trading unbounded memory for
// avoiding per-call allocation. The benchmark establishes the baseline cost
// of the current approach (allocate every time) under this workload.
func BenchmarkRecordProcessing_HighCardinality(b *testing.B) {
	pm := newNoopPipeMetrics(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		// Simulate a unique label per message (e.g. order ID, trace ID).
		info := pipe.ProcessingInfo{
			Labels: map[string]string{
				"pipeline": "orders",
				"stage":    "router",
				"order.id": string(rune('A' + i%26)), // bounded but representative
			},
			Duration:    time.Millisecond,
			OutputCount: 1,
		}
		pm.RecordProcessing(ctx, info)
	}
}

// errBench is a package-level error so its allocation does not appear in
// the per-iteration count.
var errBench = &benchErr{}

type benchErr struct{}

func (*benchErr) Error() string { return "bench error" }
