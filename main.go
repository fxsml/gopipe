package gopipe

// import (
// 	"context"
// 	"log/slog"
// 	"os"
// 	"time"

// 	"github.com/fxsml/gopipe/log"
// )

// func main() {
// 	ctx := context.Background()

// 	// Set up a base logger
// 	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
// 		Level: slog.LevelDebug,
// 	}))

// 	// 1️⃣ Fine-grained logger: logs every metric event (trace-style)
// 	singleMetrics := log.NewSingleMetricsProvider(logger, slog.LevelDebug, "orders-pipeline")

// 	// Simulated input
// 	in := make(chan int, 10)
// 	for i := 0; i < 10; i++ {
// 		in <- i
// 	}
// 	close(in)

// 	// Process using both metrics providers
// 	out := Process(
// 		ctx,
// 		in,
// 		func(ctx context.Context, x int) (int, error) {
// 			time.Sleep(200 * time.Millisecond)
// 			return x * 2, nil
// 		},
// 		func(x int, err error) {
// 			logger.Error("cancelled", "item", x, "err", err)
// 		},
// 		WithMetrics(singleMetrics),
// 	)

// 	for v := range out {
// 		logger.Info("processed result", "value", v)
// 	}

// 	// Stop background metric logging
// 	aggregateMetrics.Stop()
// }
