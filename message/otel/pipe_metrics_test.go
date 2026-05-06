package otel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	gopelotel "github.com/fxsml/gopipe/message/otel"
	"github.com/fxsml/gopipe/pipe"
)

func TestNewPipeMetrics(t *testing.T) {
	t.Parallel()
	_, provider := newTestProvider(t)
	pm, err := gopelotel.NewPipeMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewPipeMetrics() error: %v", err)
	}
	if pm == nil {
		t.Fatal("want non-nil *PipeMetrics")
	}
}

func TestPipeMetrics_RecordProcessing_Success(t *testing.T) {
	t.Parallel()
	reader, provider := newTestProvider(t)
	pm, err := gopelotel.NewPipeMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewPipeMetrics(): %v", err)
	}

	pm.RecordProcessing(context.Background(), pipe.ProcessingInfo{
		Labels:      map[string]string{"stage": "router"},
		Duration:    50 * time.Millisecond,
		OutputCount: 2,
	})

	// Success path: process.duration must be recorded.
	m, ok := collectMetric(t, reader, "gopipe.process.duration")
	if !ok {
		t.Fatal("gopipe.process.duration not found on success path")
	}
	if got := histogramCount(t, m); got != 1 {
		t.Errorf("want process.duration count=1, got %d", got)
	}

	// process.output counter must reflect OutputCount.
	mo, ok := collectMetric(t, reader, "gopipe.process.output")
	if !ok {
		t.Fatal("gopipe.process.output not found on success path")
	}
	if got := sumInt64(t, mo); got != 2 {
		t.Errorf("want process.output=2, got %d", got)
	}

	// process.total must be incremented with error_type="" on success.
	mt, ok := collectMetric(t, reader, "gopipe.process.total")
	if !ok {
		t.Fatal("gopipe.process.total not found on success path")
	}
	if got := sumInt64(t, mt); got != 1 {
		t.Errorf("want process.total=1, got %d", got)
	}
	sum := mt.Data.(metricdata.Sum[int64])
	for _, dp := range sum.DataPoints {
		for _, attr := range dp.Attributes.ToSlice() {
			if string(attr.Key) == "error.type" && attr.Value.AsString() != "" {
				t.Errorf("want error.type=\"\" on success, got %q", attr.Value.AsString())
			}
		}
	}
}

func TestPipeMetrics_RecordProcessing_Error(t *testing.T) {
	t.Parallel()
	reader, provider := newTestProvider(t)
	pm, err := gopelotel.NewPipeMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewPipeMetrics(): %v", err)
	}

	boom := errors.New("boom")
	pm.RecordProcessing(context.Background(), pipe.ProcessingInfo{
		Duration: 10 * time.Millisecond,
		Error:    boom,
	})

	// process.total must be incremented with non-empty error.type on failure.
	m, ok := collectMetric(t, reader, "gopipe.process.total")
	if !ok {
		t.Fatal("gopipe.process.total not found on error path")
	}
	if got := sumInt64(t, m); got != 1 {
		t.Errorf("want process.total=1, got %d", got)
	}

	// error.type attribute must be present and non-empty.
	sum := m.Data.(metricdata.Sum[int64])
	found := false
	for _, dp := range sum.DataPoints {
		for _, attr := range dp.Attributes.ToSlice() {
			if string(attr.Key) == "error.type" && attr.Value.AsString() != "" {
				found = true
			}
		}
	}
	if !found {
		t.Error("want non-empty error.type attribute on error data point")
	}
}

func TestPipeMetrics_RecordWait_Receive(t *testing.T) {
	t.Parallel()
	reader, provider := newTestProvider(t)
	pm, err := gopelotel.NewPipeMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewPipeMetrics(): %v", err)
	}

	pm.RecordWait(context.Background(), pipe.WaitInfo{
		Operation: "receive",
		Duration:  5 * time.Millisecond,
	})

	if _, ok := collectMetric(t, reader, "gopipe.receive.wait.duration"); !ok {
		t.Error("gopipe.receive.wait.duration not found")
	}
	if _, ok := collectMetric(t, reader, "gopipe.send.wait.duration"); ok {
		// send histogram may be registered but must have no data points.
		m, _ := collectMetric(t, reader, "gopipe.send.wait.duration")
		hist, _ := m.Data.(metricdata.Histogram[float64])
		var total uint64
		for _, dp := range hist.DataPoints {
			total += dp.Count
		}
		if total != 0 {
			t.Error("send-wait histogram must have no data points after receive-only call")
		}
	}
}

func TestPipeMetrics_RecordWait_Send(t *testing.T) {
	t.Parallel()
	reader, provider := newTestProvider(t)
	pm, err := gopelotel.NewPipeMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewPipeMetrics(): %v", err)
	}

	pm.RecordWait(context.Background(), pipe.WaitInfo{
		Operation: "send",
		Duration:  3 * time.Millisecond,
	})

	if _, ok := collectMetric(t, reader, "gopipe.send.wait.duration"); !ok {
		t.Error("gopipe.send.wait.duration not found")
	}
}

func TestPipeMetrics_Labels(t *testing.T) {
	t.Parallel()
	reader, provider := newTestProvider(t)
	pm, err := gopelotel.NewPipeMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewPipeMetrics(): %v", err)
	}

	labels := map[string]string{"pipeline": "orders", "stage": "router"}
	pm.RecordWait(context.Background(), pipe.WaitInfo{
		Labels:    labels,
		Operation: pipe.WaitOpReceive,
		Duration:  1 * time.Millisecond,
	})

	m, ok := collectMetric(t, reader, "gopipe.receive.wait.duration")
	if !ok {
		t.Fatal("gopipe.receive.wait.duration not found")
	}
	hist := m.Data.(metricdata.Histogram[float64])
	if len(hist.DataPoints) == 0 {
		t.Fatal("no data points")
	}
	attrMap := map[string]string{}
	for _, attr := range hist.DataPoints[0].Attributes.ToSlice() {
		attrMap[string(attr.Key)] = attr.Value.AsString()
	}
	if attrMap["pipeline"] != "orders" {
		t.Errorf("want pipeline=orders, got %q", attrMap["pipeline"])
	}
	if attrMap["stage"] != "router" {
		t.Errorf("want stage=router, got %q", attrMap["stage"])
	}
}
