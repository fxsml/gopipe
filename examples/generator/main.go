package main

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/channel"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Microsecond)
	defer cancel()

	i := 0
	gen := pipe.NewGenerator(func(ctx context.Context) ([]int, error) {
		defer func() { i += 5 }()
		return []int{i, i + 1, i + 2, i + 3, i + 4}, nil
	})

	<-channel.Sink(gen.Generate(ctx), func(v int) {
		println(v)
	})
}
