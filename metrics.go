package gopipe

import (
	"context"
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

// MetricsCollector defines a function that collects single input metrics.
type MetricsCollector func(metrics *Metrics)

// UseMetrics creates a middleware that collects processing metrics for each input.
// It records the start time, duration, and output count for each processing call.
func UseMetrics[In, Out any](collect MetricsCollector) MiddlewareFunc[In, Out] {
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

func NewMetricsDistributor(collectors ...MetricsCollector) MetricsCollector {
	return func(m *Metrics) {
		for _, c := range collectors {
			c(m)
		}
	}
}
