package main

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/channel"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create FanIn with buffered output and shutdown timeout
	fanin := pipe.NewFanIn[string](pipe.FanInConfig{
		Buffer:          10,
		ShutdownTimeout: 500 * time.Millisecond,
	})

	// Register input channels
	// NOTE: Errors are ignored since fanin is not closed yet
	fanin.Add(channel.FromValues("A", "B", "C"))
	fanin.Add(channel.FromValues("1", "2", "3"))
	fanin.Add(channel.FromValues("!", "@", "#"))

	// Start
	out := fanin.Start(ctx)

	// Stop
	cancel()

	// Consume merged output
	<-channel.Sink(out, func(val string) {
		fmt.Printf("%s ", val)
	})
	fmt.Println("\nDone!")
}
