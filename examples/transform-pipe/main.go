package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create input channel with string representations of integers
	in := channel.Transform(channel.FromRange(20), func(i int) string {
		return strconv.Itoa(i)
	})

	// Create a transform pipe that converts strings to integers
	pipe := gopipe.NewTransformPipe(
		func(ctx context.Context, val string) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return strconv.Atoi(val)
		},
		gopipe.WithConcurrency[string, int](5), // 5 workers
		gopipe.WithBuffer[string, int](10),     // Buffer up to 10 results
		gopipe.WithRecover[string, int](),      // Recover from panics
	)

	// Start the pipe
	processed := pipe.Start(ctx, in)

	// Consume processed values
	<-channel.Sink(processed, func(val int) {
		fmt.Printf("Processed: %d\n", val)
	})
}
