// Package otel provides OpenTelemetry instrumentation for gopipe pipelines.
//
// # Push metrics
//
// [NewPipeMetrics] implements [pipe.Metrics] for push-based instrumentation:
// channel blocking time (receive/send), processing duration, output count,
// and a total counter with an error_type dimension (empty string on success).
// Assign to [pipe.Config.Metrics], [message.PipeConfig.Metrics],
// [message.MergerConfig.Metrics], or [message.DistributorConfig.Metrics].
//
// When used with the message package, the default [message.PipeConfig.LabelFunc]
// automatically tags all events with "cloudevents.type".
//
// # Pull metrics (buffer depth and inflight)
//
// Call [PipeMetrics.ObserveStats] with any [StatsProvider] to register
// observable gauges for buffer depth, capacity, and inflight count.
// OTel polls the callback on each collection cycle (e.g. every Prometheus scrape),
// reading [pipe.Stats] once per cycle.
// The [StatsProvider] interface is satisfied by [message.Router], [message.Merger],
// [message.Distributor], [pipe.ProcessPipe], [pipe.BatchPipe], and
// [pipe.GeneratePipe].
package otel
