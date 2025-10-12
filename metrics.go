package gopipe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	InputBufferLen  int
	OutputBufferLen int
	StartTime       time.Time
	Duration        time.Duration
	Success         int32
	Failure         int32
	Cancelled       int32
	InFlight        int32
}

func newProcessMetric(inFlight *atomic.Int32, inputBufferLen int) *Metrics {
	return &Metrics{
		InFlight:       inFlight.Add(1),
		InputBufferLen: inputBufferLen,
		StartTime:      time.Now(),
	}
}

func newCancelMetric(inputBufferLen, outputBufferLen int) *Metrics {
	now := time.Now()
	return &Metrics{
		Cancelled:       1,
		InputBufferLen:  inputBufferLen,
		OutputBufferLen: outputBufferLen,
		StartTime:       now,
		Duration:        0,
	}
}

func (m *Metrics) failure(inFlight *atomic.Int32, outputBufferLen int) {
	inFlight.Add(-1)
	m.Failure++
	m.Duration = time.Since(m.StartTime)
	m.OutputBufferLen = outputBufferLen
}

func (m *Metrics) success(inFlight *atomic.Int32, outputBufferLen int) {
	inFlight.Add(-1)
	m.Success++
	m.Duration = time.Since(m.StartTime)
	m.OutputBufferLen = outputBufferLen
}

type metricsCollector struct {
	c []MetricsCollector
}

func newMetricsCollector(c []MetricsCollector) *metricsCollector {
	if len(c) == 0 {
		return &metricsCollector{}
	}
	return &metricsCollector{c: c}
}

func (mc *metricsCollector) collect(m *Metrics) {
	// TODO: make async with chan
	for _, c := range mc.c {
		c.Collect(*m)
	}
}

func (mc *metricsCollector) Done() {
	for _, c := range mc.c {
		c.Done()
	}
}

type MetricsCollector interface {
	Collect(m Metrics)
	Done()
}

type IntStats struct {
	Min   int64
	Max   int64
	Avg   float64
	Total int64
}

func (s *IntStats) calcAvg(count int32) {
	s.Avg = float64(s.Total) / float64(count)
}

type DurationStats struct {
	Min   time.Duration
	Max   time.Duration
	Avg   time.Duration
	Total time.Duration
}

func (s *DurationStats) calcAvg(count int32) {
	s.Avg = time.Duration(int64(float64(s.Total) / float64(count)))
}

type DeltaMetrics struct {
	DeltaStartTime  time.Time
	DeltaDuration   time.Duration
	InputBufferLen  IntStats
	OutputBufferLen IntStats
	InFlight        IntStats
	TotalSuccess    int32
	TotalFailure    int32
	TotalCancelled  int32
	Duration        DurationStats
	Count           int32
}

type deltaMetricsCollector struct {
	mu   sync.Mutex
	dm   DeltaMetrics
	done func()
}

func NewDeltaMetricsCollector(interval time.Duration, report func(dm DeltaMetrics)) MetricsCollector {
	ctx, done := context.WithCancel(context.Background())
	c := &deltaMetricsCollector{
		mu:   sync.Mutex{},
		dm:   newDeltaMetrics(),
		done: done,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				report(c.reset())
				return
			case <-time.After(interval):
				report(c.reset())
			}
		}
	}()
	return c
}

func newDeltaMetrics() DeltaMetrics {
	dm := DeltaMetrics{}
	dm.DeltaStartTime = time.Now()
	dm.InputBufferLen.Min = int64(^uint64(0) >> 1)
	dm.OutputBufferLen.Min = int64(^uint64(0) >> 1)
	dm.InFlight.Min = int64(^uint64(0) >> 1)
	dm.Duration.Min = time.Duration(int64(^uint64(0) >> 1))
	return dm
}

func (c *deltaMetricsCollector) reset() DeltaMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	new := newDeltaMetrics()
	dm := c.dm
	c.dm = new

	dm.DeltaDuration = new.DeltaStartTime.Sub(dm.DeltaStartTime)
	dm.InputBufferLen.calcAvg(dm.Count)
	dm.OutputBufferLen.calcAvg(dm.Count)
	dm.InFlight.calcAvg(dm.Count)
	dm.Duration.calcAvg(dm.Count)
	return dm
}

func (c *deltaMetricsCollector) Collect(m Metrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	inputBufferLen := int64(m.InputBufferLen)
	c.dm.InputBufferLen.Max = max(c.dm.InputBufferLen.Max, inputBufferLen)
	c.dm.InputBufferLen.Min = min(c.dm.InputBufferLen.Min, inputBufferLen)
	c.dm.InputBufferLen.Total += inputBufferLen

	outputBufferLen := int64(m.OutputBufferLen)
	c.dm.OutputBufferLen.Max = max(c.dm.OutputBufferLen.Max, outputBufferLen)
	c.dm.OutputBufferLen.Min = min(c.dm.OutputBufferLen.Min, outputBufferLen)
	c.dm.OutputBufferLen.Total += outputBufferLen

	inFlight := int64(m.InFlight)
	c.dm.InFlight.Max = max(c.dm.InFlight.Max, inFlight)
	c.dm.InFlight.Min = min(c.dm.InFlight.Min, inFlight)
	c.dm.InFlight.Total += inFlight

	c.dm.TotalSuccess += m.Success
	c.dm.TotalFailure += m.Failure
	c.dm.TotalCancelled += m.Cancelled

	duration := m.Duration
	c.dm.Duration.Min = min(c.dm.Duration.Min, duration)
	c.dm.Duration.Max = max(c.dm.Duration.Max, duration)
	c.dm.Duration.Total += duration

	c.dm.Count++
}

func (c *deltaMetricsCollector) Done() {
	c.done()
}
