package gopipe

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

)

// Metrics holds processing metrics for a single input.
type Metrics struct {
	StartTime time.Time
	Duration  time.Duration

	InputCount  int
	OutputCount int

	InFlight int

	Metadata Metadata
	Error    error
}

func (m *Metrics) Success() int {
	if m.Error == nil {
		return 1
	}
	return 0
}

func (m *Metrics) Failure() int {
	if IsFailure(m.Error) {
		return 1
	}
	return 0
}

func (m *Metrics) Cancel() int {
	if IsCancel(m.Error) {
		return 1
	}
	return 0
}

// MetricsFunc defines a function that collects single input metrics.
type MetricsFunc func(metrics *Metrics)

// UseMetrics creates a middleware that collects processing metrics for each input.
// It records the start time, duration, and output count for each processing call.
func UseMetrics[In, Out any](collect MetricsFunc) MiddlewareFunc[In, Out] {
	inFlight := atomic.Int32{}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				m := &Metrics{
					StartTime:  time.Now(),
					InputCount: 1,
					InFlight:   int(inFlight.Add(1)),
					Metadata:   MetadataFromContext(ctx),
				}

				out, err := next.Process(ctx, in)

				m.Duration = time.Since(m.StartTime)
				inFlight.Add(-1)
				m.OutputCount = len(out)
				m.Error = err

				collect(m)

				return out, err
			},
			func(in In, err error) {
				next.Cancel(in, err)
				if !IsFailure(err) {
					collect(&Metrics{
						InputCount: 1,
						InFlight:   int(inFlight.Load()),
						Metadata:   MetadataFromError(err),
						Error:      err,
					})
				}
			},
		)
	}
}

// NewMetricsDistributor creates a CollectMetricsFunc that distributes collected metrics.
func NewMetricsDistributor(ctx context.Context, collectors ...MetricsFunc) MetricsFunc {
	ch := make(chan *Metrics, len(collectors)*100)

	go func() {
		for metrics := range ch {
			for _, collect := range collectors {
				collect(metrics)
			}
		}
	}()

	once := sync.Once{}
	return func(metrics *Metrics) {
		if ctx.Err() != nil {
			once.Do(func() {
				close(ch)
			})
			return
		}
		ch <- metrics
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

// DeltaMetrics holds aggregated metrics over a period.
type DeltaMetrics struct {
	StartTime time.Time
	Duration  time.Duration

	Total int

	DurationStats DurationStats

	InputCountTotal  int
	OutputCountTotal int

	InFlightStats Stats

	SuccessTotal int
	FailureTotal int
	CancelTotal  int
}

// DeltaMetricsFunc defines a function that collects delta metrics.
type DeltaMetricsFunc func(metrics *DeltaMetrics)

// NewDeltaMetricsCollector creates a DeltaMetricsFunc that aggregates metrics into delta metrics.
func NewDeltaMetricsCollector(
	ctx context.Context,
	collect DeltaMetricsFunc,
	maxSize int,
	maxDuration time.Duration,
) MetricsFunc {
	ch := make(chan *Metrics, 10)

	startTime := time.Now()

	Batch(
		ch,
		func(batch []*Metrics) []struct{} {
			now := time.Now()
			batchSize := len(batch)
			dm := &DeltaMetrics{
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

				dm.InputCountTotal += m.InputCount
				dm.OutputCountTotal += m.OutputCount

				if m.Error == nil {
					dm.SuccessTotal++
				} else if IsFailure(m.Error) {
					dm.FailureTotal++
				} else {
					dm.CancelTotal++
				}
			}

			dm.InFlightStats.Avg = float64(inFlightTotal) / float64(batchSize)
			dm.DurationStats.Avg = durationTotal / time.Duration(batchSize)

			startTime = now
			collect(dm)
			return nil
		},
		maxSize,
		maxDuration,
	)

	once := sync.Once{}
	return func(m *Metrics) {
		if ctx.Err() != nil {
			once.Do(func() {
				close(ch)
			})
			return
		}
		ch <- m
	}
}
