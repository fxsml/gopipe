// Example 08: OTel metrics — push instruments + pull Stats() bridged to gauges.
//
// Wires a single Router (no Engine, no Merger, no Distributor).
// A handler sleeps for a random 0–50 ms to produce visible timing histograms.
//
// Push metrics (auto-recorded by gopipe):
//   - gopipe.receive.wait.duration   time blocked dequeuing from input
//   - gopipe.send.wait.duration      time blocked enqueuing to output
//   - gopipe.process.duration        handler execution time, tagged by CE type
//   - gopipe.process.output          messages emitted by handler, tagged by CE type
//   - gopipe.process.total           messages processed; error_type="" on success,
//     Go error type on failure — query errors with {error_type!=""}
//
// Pull metrics (registered via pm.ObserveStats, polled on each Prometheus scrape):
//   - gopipe.buffer.depth      router output buffer occupancy
//   - gopipe.buffer.capacity   router output buffer capacity (denominator for utilisation)
//   - gopipe.inflight          active worker count
//
// Scrape: curl http://localhost:2112/metrics
// Run:    go run ./examples/08-otel-metrics
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	prometheusexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	msgotel "github.com/fxsml/gopipe/message/otel"
)

// ProcessOrder is the handler input type
type ProcessOrder struct {
	OrderID string
}

// ProcessedOrder is the handler output type
type ProcessedOrder struct {
	OrderID string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── OTel: Prometheus exporter serves /metrics ─────────────────────────────
	exporter, err := prometheusexporter.New()
	if err != nil {
		log.Fatal("prometheus exporter:", err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("gopipe.example")

	// ── Push: PipeMetrics wired into the router pipe config ───────────────────
	//
	// Records channel wait durations, processing duration, and errors automatically.
	// The message package default LabelFunc tags all events with cloudevents.type.
	pm, err := msgotel.NewPipeMetrics(meter)
	if err != nil {
		log.Fatal("NewPipeMetrics:", err)
	}

	// Static labels shared by both the pipe config and ObserveStats.
	// Declaring them once ensures push and pull instruments use the same label set.
	pipeLabels := map[string]string{
		"pipeline": "orders",
		"stage":    "router",
	}

	// ── Router: 2 workers, buffer 64 ─────────────────────────────────────────
	//
	// The producer sends faster than handlers finish on average (10 ms/msg in,
	// 0–50 ms/msg to handle across 2 workers), so the output buffer accumulates
	// a backlog — making queue depth and send-wait visible in the scrape.
	router := message.NewRouter(message.PipeConfig{
		Pool:    message.PoolConfig{Workers: 2, BufferSize: 64},
		Metrics: pm,
		Labels:  pipeLabels,
	})

	// ── Pull: ObserveStats() → OTel observable gauges ─────────────────────────
	//
	// Registers buffer depth and inflight gauges. Labels are read from Stats()
	// at collection time — same source as push labels — ensuring consistent
	// label sets without coupling PipeMetrics to the pipe configuration.
	if err := pm.ObserveStats(router); err != nil {
		log.Fatal("ObserveStats:", err)
	}

	// ── Handler ───────────────────────────────────────────────────────────────
	//
	// Random 0–50 ms sleep produces a spread across histogram buckets in
	// gopipe_message_process_duration. Returns no output events.
	if err := router.AddHandler("process-order", nil, message.NewCommandHandler(
		func(_ context.Context, msg ProcessOrder) ([]ProcessedOrder, error) {
			time.Sleep(time.Duration(rand.IntN(50)) * time.Millisecond)
			return []ProcessedOrder{{OrderID: msg.OrderID}}, nil
		},
		message.CommandHandlerConfig{
			Source: "/orders",
			Naming: message.DotNaming, // ProcessOrder → "process.order"
		},
	)); err != nil {
		log.Fatal("AddHandler:", err)
	}

	// ── Wire ──────────────────────────────────────────────────────────────────
	//
	// FromFunc drives the producer: one message every 10 ms.
	// With 2 workers averaging 25 ms each the pipeline is under-provisioned on
	// purpose so the queue fills visibly.
	var i int
	input := channel.FromFunc(ctx, func() *message.Message {
		defer func() { i++ }()
		time.Sleep(10 * time.Millisecond)
		return message.New(
			&ProcessOrder{OrderID: fmt.Sprintf("ORD-%06d", i)},
			message.Attributes{
				message.AttrType:        "process.order",
				message.AttrSpecVersion: "1.0",
				message.AttrSource:      "/orders",
			},
			nil,
		)
	})

	out, err := router.Pipe(ctx, input)
	if err != nil {
		log.Fatal("router.Pipe:", err)
	}
	channel.Drain(out) // handler returns nil; discard the empty output

	// ── Serve ─────────────────────────────────────────────────────────────────
	http.Handle("/metrics", promhttp.Handler())
	log.Println("Metrics at http://localhost:2112/metrics  (Ctrl+C to stop)")
	log.Fatal(http.ListenAndServe(":2112", nil))
}
