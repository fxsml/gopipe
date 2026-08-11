package pipe

import (
	"context"
	"maps"
	"time"
)

// Metrics receives pipeline observability events.
// All methods must be safe for concurrent use.
// A nil Metrics value is valid and results in no instrumentation overhead.
//
// Buffer depth is not reported via this interface. Instead, call Stats() on
// [ProcessPipe], [Merger], or [Distributor] and observe the returned [Stats]
// using your metrics backend's pull/observable mechanism (e.g. OTel observable gauge).
type Metrics interface {
	// RecordProcessing is called after each item is processed.
	RecordProcessing(ctx context.Context, info ProcessingInfo)

	// RecordWait is called after blocking on a channel operation.
	RecordWait(ctx context.Context, info WaitInfo)
}

// ProcessingInfo contains metrics for a single processed item.
type ProcessingInfo struct {
	// Labels are the static and dynamic labels for this event.
	// Must not be modified by the receiver.
	Labels map[string]string

	// Duration is the handler execution time.
	Duration time.Duration

	// Error is the processing error, or nil on success.
	Error error

	// OutputCount is the number of output items produced.
	OutputCount int
}

// Operation constants for [WaitInfo.Operation].
const (
	// WaitOpReceive is used when a worker was blocked waiting for the next input item.
	WaitOpReceive = "receive"

	// WaitOpSend is used when a worker was blocked sending an output item downstream.
	WaitOpSend = "send"
)

// WaitInfo contains metrics for a channel blocking event.
type WaitInfo struct {
	// Labels are the static and dynamic labels for this event.
	// Must not be modified by the receiver.
	Labels map[string]string

	// Operation is [WaitOpReceive] when blocked waiting for input,
	// or [WaitOpSend] when blocked sending to an output channel.
	Operation string

	// Duration is the time spent blocked on the channel operation.
	Duration time.Duration
}

// mergeLabels returns the static labels merged with the dynamic labels produced
// by calling fn(val). If fn is nil or returns nothing, static is returned as-is
// (no allocation). The returned map must not be modified by the receiver.
func mergeLabels(static map[string]string, fn func(any) map[string]string, val any) map[string]string {
	if fn == nil {
		return static
	}
	dynamic := fn(val)
	if len(dynamic) == 0 {
		return static
	}
	merged := make(map[string]string, len(static)+len(dynamic))
	maps.Copy(merged, static)
	maps.Copy(merged, dynamic)
	return merged
}

// Stats is a point-in-time snapshot of an output channel buffer.
// Returned by Stats() on [ProcessPipe], [Merger], and [Distributor].
// Intended for use with pull-based observability (e.g. OTel observable gauge):
// register a callback that calls Stats() at collection time rather than
// recording on every send.
type Stats struct {
	// Depth is the current number of items in the buffer (len).
	Depth int

	// Capacity is the total buffer size (cap).
	Capacity int

	// Inflight is the number of messages currently being processed by workers.
	// Zero for components that do not execute user handlers (e.g. Merger, Distributor).
	Inflight int

	// Labels are the static labels configured on the component (pipe.Config.Labels).
	// Same source as ProcessingInfo.Labels and WaitInfo.Labels, enabling consistent
	// label sets across push and pull metrics without coupling to the metrics backend.
	Labels map[string]string
}
