# gopipeline

A lightweight, generic Go library for building concurrent, composable data pipelines using channels and context. Supports processing, filtering, batching, broadcasting, merging, and draining of channel data with robust error handling and cancellation.

## Features
- **Generic API**: Works with any data type (Go 1.18+).
- **Concurrent Processing**: Easily control concurrency and buffering.
- **Batch Processing**: Group items for efficient batch operations.
- **Filtering**: Select items based on custom logic.
- **Broadcast & Merge**: Fan-out and fan-in channel data.
- **Graceful Cancellation**: Context-aware for safe shutdowns.
- **Custom Error Handling**: Flexible error reporting for both single and batch operations.

## Installation

```
go get github.com/fxsml/gopipeline
```

## Usage

### Process Example

```go
package main

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipeline"
)

func main() {
	ctx := context.Background()
	in := make(chan int)

	go func() {
		defer close(in)
		for i := 1; i <= 5; i++ {
			in <- i
		}
	}()

	out := gopipeline.Process(ctx, in, func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	}, gopipeline.WithConcurrency(2))

	for v := range out {
		fmt.Println(v)
	}
}
```

### ProcessBatch Example

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fxsml/gopipeline"
)

func main() {
	ctx := context.Background()
	in := make(chan int)
	go func() {
		for i := 1; i <= 6; i++ {
			in <- i
		}
		close(in)
	}()

	handler := func(ctx context.Context, batch []int) ([]gopipeline.BatchResult[int], error) {
		res := make([]gopipeline.BatchResult[int], len(batch))
		for i, v := range batch {
			if v%4 == 0 {
				res[i] = gopipeline.NewBatchResult(0, fmt.Errorf("rejecting %d", v))
			} else {
				res[i] = gopipeline.NewBatchResult(v*10, nil)
			}
		}
		return res, nil
	}

	out := gopipeline.ProcessBatch(
		ctx,
		in,
		handler,
		gopipeline.HandleBatchResult,
		3,
		2*time.Second,
		gopipeline.WithErrorHandler(func(val any, err error) {
			switch {
			case errors.Is(err, gopipeline.ErrProcessBatch):
				fmt.Printf("error: '%v', batch: '%v'\n", err, val)
			case errors.Is(err, gopipeline.ErrProcessBatchResult):
				fmt.Printf("error: '%v', item: '%v'\n", err, val)
			}
		}),
	)

	for v := range out {
		fmt.Println(v)
	}
}
```
