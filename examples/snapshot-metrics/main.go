package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	in := channel.FromRange(10)

	gopipe.SetDefaultLogConfig(&gopipe.LogConfig{
		LevelSuccess: "info",
	})

	sm, done := gopipe.NewSnapshotMetricsCollector(ctx, func(metrics *gopipe.SnapshotMetrics) {
		fmt.Printf("Snapshot Metrics: Success=%d, Cancelled=%d, Failed=%d, Retried=%d\n",
			metrics.SuccessTotal,
			metrics.CancelTotal,
			metrics.FailureTotal,
			metrics.RetryTotal,
		)
	}, 100, 5*time.Second)

	defer func() {
		cancel()
		<-done
	}()

	pipe := gopipe.NewTransformPipe(func(ctx context.Context, v int) (string, error) {
		if v%2 == 0 {
			return "", errors.New("gotcha")
		}
		return fmt.Sprintf("Number: %d", v), nil
	},
		gopipe.WithMetadataProvider[int, string](func(in int) gopipe.Metadata {
			return gopipe.Metadata{
				"input_value": in,
			}
		}),
		gopipe.WithRetryConfig[int, string](&gopipe.RetryConfig{
			Backoff: gopipe.ConstantBackoff(1*time.Second, 0),
		}),
		gopipe.WithMetricsCollector[int, string](sm),
	)

	out := pipe.Start(context.Background(), in)

	<-channel.Drain(out)
	time.Sleep(1 * time.Second)
}
