package gopipe

//import (
//	"context"
//	"math"
//	"time"
//)
//
//// Stats holds statistical data.
//type Stats struct {
//	Min int
//	Max int
//	Avg float64
//}
//
//// DurationStats holds duration-based statistical data.
//type DurationStats struct {
//	Min time.Duration
//	Max time.Duration
//	Avg time.Duration
//}
//
//// SnapshotMetrics holds aggregated metrics over a period.
//type SnapshotMetrics struct {
//	StartTime time.Time
//	Duration  time.Duration
//
//	Total int
//
//	DurationStats DurationStats
//
//	InputCountTotal  int
//	OutputCountTotal int
//
//	InFlightStats Stats
//
//	SuccessTotal int
//	FailureTotal int
//	CancelTotal  int
//}
//
//// SnapshotMetricsFunc defines a function that collects snapshot metrics.
//type SnapshotMetricsFunc func(metrics *SnapshotMetrics)
//
//// NewSnapshotMetricsCollector creates a MetricsCollector that aggregates metrics into snapshot metrics.
//func NewSnapshotMetricsCollector(
//	ctx context.Context,
//	collect SnapshotMetricsFunc,
//	maxSize int,
//	maxDuration time.Duration,
//) (MetricsCollector, <-chan struct{}) {
//	ch := make(chan *Metrics)
//
//	startTime := time.Now()
//
//	batchCh := Collect(
//		Cancel(ctx, ch, func(_ *Metrics, _ error) {}),
//		maxSize,
//		maxDuration,
//	)
//
//	out := Sink(batchCh, func(batch []*Metrics) {
//		now := time.Now()
//		batchSize := len(batch)
//		dm := &SnapshotMetrics{
//			StartTime: startTime,
//			Duration:  startTime.Sub(now),
//			Total:     batchSize,
//			InFlightStats: Stats{
//				Min: math.MaxInt64,
//			},
//			DurationStats: DurationStats{
//				Min: math.MaxInt64,
//			},
//		}
//		inFlightTotal := 0
//		durationTotal := time.Duration(0)
//		for _, m := range batch {
//			inFlightTotal += m.InFlight
//			dm.InFlightStats.Max = max(dm.InFlightStats.Max, m.InFlight)
//			dm.InFlightStats.Min = min(dm.InFlightStats.Min, m.InFlight)
//
//			durationTotal += m.Duration
//			dm.DurationStats.Max = max(dm.DurationStats.Max, m.Duration)
//			dm.DurationStats.Min = min(dm.DurationStats.Min, m.Duration)
//
//			dm.InputCountTotal += m.InputCount
//			dm.OutputCountTotal += m.OutputCount
//
//			if m.Error == nil {
//				dm.SuccessTotal++
//			} else if IsFailure(m.Error) {
//				dm.FailureTotal++
//			} else {
//				dm.CancelTotal++
//			}
//		}
//
//		dm.InFlightStats.Avg = float64(inFlightTotal) / float64(batchSize)
//		dm.DurationStats.Avg = durationTotal / time.Duration(batchSize)
//
//		startTime = now
//		collect(dm)
//	})
//
//	return func(m *Metrics) {
//		select {
//		case <-ctx.Done():
//		case ch <- m:
//		}
//	}, out
//}
//
//// NewNonBlockingMetricsDistributor creates a CollectMetricsFunc that distributes collected metrics.
//func NewNonBlockingMetricsDistributor(ctx context.Context, collectors ...MetricsCollector) (MetricsCollector, <-chan struct{}) {
//	ch := make(chan *Metrics)
//	out := make(chan struct{})
//
//	go func() {
//		defer close(out)
//		for {
//			select {
//			case <-ctx.Done():
//				close(ch)
//				return
//			case metrics := <-ch:
//				for _, collect := range collectors {
//					collect(metrics)
//				}
//			}
//		}
//	}()
//
//	return func(metrics *Metrics) {
//		select {
//		case <-ctx.Done():
//		case ch <- metrics:
//		}
//	}, out
//}
