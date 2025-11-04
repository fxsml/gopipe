package gopipe

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"time"

	"github.com/fxsml/gopipe/channel"
)

// Metrics holds processing metrics for a single input.
type Metrics struct {
	Start    time.Time
	Duration time.Duration
	Input    int
	Output   int
	InFlight int

	Metadata   Metadata
	RetryState *RetryState

	Error error
}

// Success returns a numeric indicator of success (1 for success, 0 otherwise).
func (m *Metrics) Success() int {
	if m.Error == nil {
		return 1
	}
	return 0
}

// Failure returns a numeric indicator of failure (1 for failure, 0 otherwise).
func (m *Metrics) Failure() int {
	if errors.Is(m.Error, ErrFailure) {
		return 1
	}
	return 0
}

// Cancel returns a numeric indicator of cancellation (1 for cancel, 0 otherwise).
func (m *Metrics) Cancel() int {
	if errors.Is(m.Error, ErrCancel) {
		return 1
	}
	return 0
}

// Retry returns a numeric indicator of retry (1 for retry, 0 otherwise).
func (m *Metrics) Retry() int {
	if errors.Is(m.Error, ErrRetry) {
		return 1
	}
	return 0
}

// MetricsCollector defines a function that collects single input metrics.
type MetricsCollector func(metrics *Metrics)

// WithMetricsCollector adds a metrics collector to the processing pipeline.
// Can be used multiple times to add multiple collectors.
func WithMetricsCollector[In, Out any](collector MetricsCollector) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.metricsCollector = append(cfg.metricsCollector, collector)
	}
}

// Stats holds statistical data.
type Stats struct {
	Min int
	Max int
	Avg float64
}

// DurationStats holds duration-based statistical data.
type DurationStats struct {
	Min time.Duration
	Max time.Duration
	Avg time.Duration
}

// SnapshotMetrics holds aggregated metrics over a period.
type SnapshotMetrics struct {
	StartTime time.Time
	Duration  time.Duration

	Total int

	DurationStats DurationStats

	InputTotal  int
	OutputTotal int

	InFlightStats Stats

	SuccessTotal int
	FailureTotal int
	CancelTotal  int
	RetryTotal   int
}

// SnapshotMetricsCollector defines a function that collects snapshot metrics.
type SnapshotMetricsCollector func(metrics *SnapshotMetrics)

// NewSnapshotMetricsCollector creates a MetricsCollector that aggregates metrics into snapshot metrics.
func NewSnapshotMetricsCollector(
	ctx context.Context,
	collect SnapshotMetricsCollector,
	maxSize int,
	maxDuration time.Duration,
) (MetricsCollector, <-chan struct{}) {
	ch := make(chan *Metrics)

	startTime := time.Now()

	batchCh := channel.Collect(
		channel.Cancel(ctx, ch, func(_ *Metrics, _ error) {}),
		maxSize,
		maxDuration,
	)

	out := channel.Sink(batchCh, func(batch []*Metrics) {
		now := time.Now()
		batchSize := len(batch)
		dm := &SnapshotMetrics{
			StartTime: startTime,
			Duration:  startTime.Sub(now),
			Total:     batchSize,
			InFlightStats: Stats{
				Min: math.MaxInt64,
			},
			DurationStats: DurationStats{
				Min: math.MaxInt64,
			},
		}
		inFlightTotal := 0
		durationTotal := time.Duration(0)
		for _, m := range batch {
			inFlightTotal += m.InFlight
			dm.InFlightStats.Max = max(dm.InFlightStats.Max, m.InFlight)
			dm.InFlightStats.Min = min(dm.InFlightStats.Min, m.InFlight)

			durationTotal += m.Duration
			dm.DurationStats.Max = max(dm.DurationStats.Max, m.Duration)
			dm.DurationStats.Min = min(dm.DurationStats.Min, m.Duration)

			dm.InputTotal += m.Input
			dm.OutputTotal += m.Output

			dm.SuccessTotal += m.Success()
			dm.FailureTotal += m.Failure()
			dm.CancelTotal += m.Cancel()
			dm.RetryTotal += m.Retry()
		}

		dm.InFlightStats.Avg = float64(inFlightTotal) / float64(batchSize)
		dm.DurationStats.Avg = durationTotal / time.Duration(batchSize)

		startTime = now
		collect(dm)
	})

	return func(m *Metrics) {
		select {
		case <-ctx.Done():
		case ch <- m:
		}
	}, out
}

func useMetrics[In, Out any](collect MetricsCollector) MiddlewareFunc[In, Out] {
	inFlight := atomic.Int32{}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				m := &Metrics{
					Start:      time.Now(),
					Input:      1,
					InFlight:   int(inFlight.Add(1)),
					Metadata:   MetadataFromContext(ctx),
					RetryState: RetryStateFromContext(ctx),
				}

				out, err := next.Process(ctx, in)

				m.Duration = time.Since(m.Start)
				inFlight.Add(-1)
				m.Output = len(out)
				m.Error = err

				if m.RetryState != nil {
					m.RetryState.Duration = time.Since(m.RetryState.Start)
				}

				collect(m)

				return out, err
			},
			func(in In, err error) {
				next.Cancel(in, err)
				if !errors.Is(err, ErrFailure) || errors.Is(err, ErrRetry) {
					collect(&Metrics{
						Input:      1,
						InFlight:   int(inFlight.Load()),
						Metadata:   MetadataFromError(err),
						RetryState: RetryStateFromError(err),
						Error:      err,
					})
				}
			},
		)
	}
}

func newMetricsDistributor(collectors ...MetricsCollector) MetricsCollector {
	return func(m *Metrics) {
		for _, c := range collectors {
			c(m)
		}
	}
}
