package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create input channel with string representations of integers
	in := channel.Transform(channel.FromRange(20), func(i int) string {
		return strconv.Itoa(i)
	})

	// Create a transform p that converts strings to integers
	p := pipe.NewTransformPipe(
		func(ctx context.Context, val string) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return strconv.Atoi(val)
		},
		pipe.Config{
			Concurrency: 5,
			BufferSize:  10,
		},
	)
	if err := p.Use(middleware.Recover[string, int]()); err != nil {
		panic(err)
	}

	// Start the pipe
	processed, err := p.Pipe(ctx, in)
	if err != nil {
		panic(err)
	}

	// Consume processed values
	<-channel.Sink(processed, func(val int) {
		fmt.Printf("Processed: %d\n", val)
	})
}
