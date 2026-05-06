package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/fxsml/gopipe/pipe"
)

// PipeMetrics implements [pipe.Metrics] using OpenTelemetry instruments.
// It records channel wait times, processing duration, and errors at the pipe layer —
// metrics that middleware cannot observe (wait times) or that benefit from
// per-value dynamic labels (duration with CloudEvents type dimension).
//
// Create via [NewPipeMetrics] and assign to [pipe.Config.Metrics] or the
// corresponding field on [message.PipeConfig], [message.MergerConfig], or
// [message.DistributorConfig].
//
// In the message package, [message.PipeConfig.LabelFunc] defaults to
// extracting "cloudevents.type" from each message, so process duration,
// output count, and total are automatically dimensioned by event type.
//
// Call [PipeMetrics.ObserveStats] after constructing the component to register
// observable gauges for buffer depth and inflight count. Labels for the gauges
// are read from [pipe.Stats.Labels] at collection time — the same source as
// push metric labels — ensuring consistent label sets without any coupling
// between the metrics backend and the pipe configuration.
type PipeMetrics struct {
	meter               metric.Meter
	receiveWaitDuration metric.Float64Histogram
	sendWaitDuration    metric.Float64Histogram
	processDuration     metric.Float64Histogram
	processTotal        metric.Int64Counter
	processOutput       metric.Int64Counter
}

// NewPipeMetrics creates a PipeMetrics backed by the provided meter.
// Returns an error if any OTel instrument cannot be created.
//
// PipeMetrics is stateless with respect to labels: all label sets flow in
// from the component at record time via [pipe.ProcessingInfo.Labels],
// [pipe.WaitInfo.Labels], and [pipe.Stats.Labels].
func NewPipeMetrics(meter metric.Meter) (*PipeMetrics, error) {
	// durationBuckets covers sub-millisecond channel operations up to 100-second handler timeouts.
	// Boundaries are in seconds (OTel convention) with resolution matching gopipe's typical range:
	// channel waits: <1ms; handler execution: 1ms–10s; worst-case timeouts: up to 100s.
	durationBuckets := metric.WithExplicitBucketBoundaries(
		0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 100.0,
	)

	receiveWait, err := meter.Float64Histogram(
		"gopipe.receive.wait.duration",
		metric.WithDescription("Time blocked waiting to receive from the input channel."),
		metric.WithUnit("s"),
		durationBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("gopipe/otel: create receive wait histogram: %w", err)
	}

	sendWait, err := meter.Float64Histogram(
		"gopipe.send.wait.duration",
		metric.WithDescription("Time blocked sending to an output channel."),
		metric.WithUnit("s"),
		durationBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("gopipe/otel: create send wait histogram: %w", err)
	}

	processDur, err := meter.Float64Histogram(
		"gopipe.process.duration",
		metric.WithDescription("Handler execution time per message."),
		metric.WithUnit("s"),
		durationBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("gopipe/otel: create process duration histogram: %w", err)
	}

	processTotal, err := meter.Int64Counter(
		"gopipe.process.total",
		metric.WithDescription("Messages processed, with error.type=\"\" on success and the Go error type on failure."),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return nil, fmt.Errorf("gopipe/otel: create process total counter: %w", err)
	}

	processOutput, err := meter.Int64Counter(
		"gopipe.process.output",
		metric.WithDescription("Number of messages emitted by the handler."),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return nil, fmt.Errorf("gopipe/otel: create process output counter: %w", err)
	}

	return &PipeMetrics{
		meter:               meter,
		receiveWaitDuration: receiveWait,
		sendWaitDuration:    sendWait,
		processDuration:     processDur,
		processTotal:        processTotal,
		processOutput:       processOutput,
	}, nil
}

// ObserveStats registers OTel observable gauges for the component's buffer depth
// and inflight count. OTel polls the callbacks on each collection cycle
// (e.g. every Prometheus scrape), reading Stats() once per cycle.
//
// Labels for the gauges are read from [pipe.Stats.Labels] at collection time —
// the same static labels that push instruments receive via [pipe.ProcessingInfo]
// and [pipe.WaitInfo] — ensuring consistent label sets across instruments.
//
// Uses a single RegisterCallback to call Stats() once per scrape, observing
// all three instruments atomically — the idiomatic OTel multi-instrument pattern.
//
// Returns an error if instrument creation or callback registration fails.
func (m *PipeMetrics) ObserveStats(s StatsProvider) error {
	depth, err := m.meter.Int64ObservableGauge(
		"gopipe.buffer.depth",
		metric.WithDescription("Messages waiting in the component output buffer."),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return fmt.Errorf("gopipe/otel: create buffer depth gauge: %w", err)
	}

	capacity, err := m.meter.Int64ObservableGauge(
		"gopipe.buffer.capacity",
		metric.WithDescription("Total capacity of the component output buffer. Use with gopipe.buffer.depth to compute utilisation."),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return fmt.Errorf("gopipe/otel: create buffer capacity gauge: %w", err)
	}

	inflight, err := m.meter.Int64ObservableGauge(
		"gopipe.inflight",
		metric.WithDescription("Messages currently being processed by workers."),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return fmt.Errorf("gopipe/otel: create inflight gauge: %w", err)
	}

	_, err = m.meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			st := s.Stats()
			attrs := metric.WithAttributes(labelsToAttrs(st.Labels)...)
			o.ObserveInt64(depth, int64(st.Depth), attrs)
			o.ObserveInt64(capacity, int64(st.Capacity), attrs)
			o.ObserveInt64(inflight, int64(st.Inflight), attrs)
			return nil
		},
		depth, capacity, inflight,
	)
	if err != nil {
		return fmt.Errorf("gopipe/otel: register stats callback: %w", err)
	}
	return nil
}

// RecordProcessing implements [pipe.Metrics].
// Records handler execution duration and output count (dimensioned by labels
// including event type when using the message package default LabelFunc).
// Increments gopipe.process.total with error_type="" on success and the
// Go error type string on failure — enabling both total and error rate queries
// without needing ignoring() vector matching.
func (m *PipeMetrics) RecordProcessing(ctx context.Context, info pipe.ProcessingInfo) {
	// Allocate one slice covering base attrs + the error.type slot, avoiding
	// the second allocation that make+copy would require for totalAttrs.
	// error.type is set before any Record/Add call so all[:n] and all share
	// the same backing array without depending on call-site synchrony.
	n := len(info.Labels)
	all := make([]attribute.KeyValue, n+1)
	i := 0
	for k, v := range info.Labels {
		all[i] = attribute.String(k, v)
		i++
	}
	errType := ""
	if info.Error != nil {
		errType = fmt.Sprintf("%T", info.Error)
	}
	all[n] = attribute.String("error.type", errType)
	attr := metric.WithAttributes(all[:n]...)
	m.processDuration.Record(ctx, info.Duration.Seconds(), attr)
	if info.OutputCount > 0 {
		m.processOutput.Add(ctx, int64(info.OutputCount), attr)
	}
	m.processTotal.Add(ctx, 1, metric.WithAttributes(all...))
}

// RecordWait implements [pipe.Metrics].
func (m *PipeMetrics) RecordWait(ctx context.Context, info pipe.WaitInfo) {
	attrs := labelsToAttrs(info.Labels)
	switch info.Operation {
	case pipe.WaitOpReceive:
		m.receiveWaitDuration.Record(ctx, info.Duration.Seconds(), metric.WithAttributes(attrs...))
	case pipe.WaitOpSend:
		m.sendWaitDuration.Record(ctx, info.Duration.Seconds(), metric.WithAttributes(attrs...))
	}
}

// labelsToAttrs converts a labels map to OTel attributes.
// Returns nil for an empty map (avoids allocation for unlabelled pipes).
func labelsToAttrs(labels map[string]string) []attribute.KeyValue {
	if len(labels) == 0 {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, len(labels))
	for k, v := range labels {
		attrs = append(attrs, attribute.String(k, v))
	}
	return attrs
}
