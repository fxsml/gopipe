# gopipe

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
go get github.com/fxsml/gopipe
```

## Usage

### Process Example

```go
package main

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipe"
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

	out := gopipe.Process(ctx, in, func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	}, gopipe.WithConcurrency(2))

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

	"github.com/fxsml/gopipe"
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

	handler := func(ctx context.Context, batch []int) ([]gopipe.BatchResult[int], error) {
		res := make([]gopipe.BatchResult[int], len(batch))
		for i, v := range batch {
			if v%4 == 0 {
				res[i] = gopipe.NewBatchResult(0, fmt.Errorf("rejecting %d", v))
			} else {
				res[i] = gopipe.NewBatchResult(v*10, nil)
			}
		}
		return res, nil
	}

	out := gopipe.ProcessBatch(
		ctx,
		in,
		handler,
		gopipe.HandleBatchResult,
		3,
		2*time.Second,
		gopipe.WithErrorHandler(func(val any, err error) {
			switch {
			case errors.Is(err, gopipe.ErrProcessBatch):
				fmt.Printf("error: '%v', batch: '%v'\n", err, val)
			case errors.Is(err, gopipe.ErrProcessBatchResult):
				fmt.Printf("error: '%v', item: '%v'\n", err, val)
			}
		}),
	)

	for v := range out {
		fmt.Println(v)
	}
}
```
