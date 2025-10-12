package gopipe

import (
	"context"
	"log/slog"
	"os"
	"time"
)

func EnableJSONLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
}

func Logger(args ...any) []MetricsCollector {
	return []MetricsCollector{
		NewMetricsLogger(slog.LevelDebug, args...),
		NewDeltaMetricsLogger(1*time.Minute, slog.LevelInfo, args...),
	}
}

type metricsLogger struct {
	logLevel slog.Level
	args     []any
}

func NewMetricsLogger(logLevel slog.Level, args ...any) MetricsCollector {
	return &metricsLogger{logLevel: logLevel, args: args}
}

func (l *metricsLogger) Collect(m Metrics) {
	args := make([]any, 0, 5+len(l.args))

	switch {
	case m.Success > 0:
		args = append(args, "result", "success")
	case m.Failure > 0:
		args = append(args, "result", "failure")
	case m.Cancelled > 0:
		args = append(args, "result", "cancelled")
	}

	args = append(args,
		"duration", m.Duration,
		"input_buffer_len", m.InputBufferLen,
		"output_buffer_len", m.OutputBufferLen,
		"in_flight", m.InFlight)

	if len(l.args) > 0 {
		args = append(args, l.args...)
	}

	slog.Log(context.Background(), l.logLevel, "gopipe metrics", args...)
}

func (l *metricsLogger) Done() {}

func NewDeltaMetricsLogger(interval time.Duration, logLevel slog.Level, args ...any) MetricsCollector {
	return NewDeltaMetricsCollector(interval, func(dm DeltaMetrics) {
		if dm.Count == 0 {
			return
		}
		args := make([]any, 0, 7+len(args))
		args = append(args,
			"total_success", dm.TotalSuccess,
			"total_failure", dm.TotalFailure,
			"total_cancelled", dm.TotalCancelled,
			"duration", dm.Duration,
			"input_buffer_len", dm.InputBufferLen,
			"output_buffer_len", dm.OutputBufferLen,
			"in_flight", dm.InFlight,
			"delta_duration", dm.DeltaDuration,
			"delta_count", dm.Count,
		)

		slog.Log(context.Background(), logLevel, "gopipe delta metrics", args...)
	})
}
