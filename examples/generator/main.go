package main

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Microsecond)
	defer cancel()

	i := 0
	gen := pipe.NewGenerator(func(ctx context.Context) ([]int, error) {
		defer func() { i += 5 }()
		return []int{i, i + 1, i + 2, i + 3, i + 4}, nil
	}, pipe.Config{})

	out, err := gen.Generate(ctx)
	if err != nil {
		panic(err)
	}

	<-channel.Sink(out, func(v int) {
		println(v)
	})
}
