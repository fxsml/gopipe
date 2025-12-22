package main

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Merger with buffered output and shutdown timeout
	merger := pipe.NewMerger[string](pipe.MergerConfig{
		Buffer:          10,
		ShutdownTimeout: 500 * time.Millisecond,
	})

	// Register input channels
	// NOTE: Errors are ignored since merger is not closed yet
	merger.Add(channel.FromValues("A", "B", "C"))
	merger.Add(channel.FromValues("1", "2", "3"))
	merger.Add(channel.FromValues("!", "@", "#"))

	// Merge
	out, err := merger.Merge(ctx)
	if err != nil {
		panic(err)
	}

	// Stop
	cancel()

	// Consume merged output
	<-channel.Sink(out, func(val string) {
		fmt.Printf("%s ", val)
	})
	fmt.Println("\nDone!")
}
