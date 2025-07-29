package gopipe

import (
	"context"
	"sync/atomic"
	"time"
)

type StatusReporter func(Status)

type Status struct {
	Received   int64
	Processed  int64
	Sent       int64
	Elapsed    time.Duration
	Rejected   int64
	LenInChan  int
	LenOutChan int
}

type atomicStatusReporter[In, Out any] struct {
	reporting bool

	received  atomic.Int64
	processed atomic.Int64
	sent      atomic.Int64
	rejected  atomic.Int64
}

func newAtomicStatusReporter[In, Out any](
	ctx context.Context,
	report StatusReporter,
	interval time.Duration,
	in <-chan In,
	out chan<- Out,
) *atomicStatusReporter[In, Out] {
	s := &atomicStatusReporter[In, Out]{}
	if report != nil && interval > 0 {
		s.reporting = true
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			start := time.Now()
			last := Status{}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					now := time.Now()
					status := Status{
						Received:   s.received.Load() - last.Received,
						Processed:  s.processed.Load() - last.Processed,
						Sent:       s.sent.Load() - last.Sent,
						Rejected:   s.rejected.Load() - last.Rejected,
						Elapsed:    now.Sub(start),
						LenInChan:  len(in),
						LenOutChan: len(out),
					}
					report(status)
					last = status
					start = now
				}
			}
		}()
	}
	return s
}

func (s *atomicStatusReporter[In, Out]) addReceived(delta int64) {
	if s.reporting {
		s.received.Add(delta)
	}
}

func (s *atomicStatusReporter[In, Out]) addProcessed(delta int64) {
	if s.reporting {
		s.processed.Add(delta)
	}
}

func (s *atomicStatusReporter[In, Out]) addSent(delta int64) {
	if s.reporting {
		s.sent.Add(delta)
	}
}

func (s *atomicStatusReporter[In, Out]) addRejected(delta int64) {
	if s.reporting {
		s.rejected.Add(delta)
	}
}
