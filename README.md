# gopipe

A lightweight, generic Go library for building composable data pipelines, message routers, and pub/sub systems.

gopipe provides three core packages for building robust, concurrent applications:

- **gopipe**: Composable pipe orchestration with concurrency, batching, and error handling
- **channel**: Channel utilities (Merge, Filter, Transform, GroupBy, etc.)
- **message**: Message handling with pub/sub, routing, CQRS, and CloudEvents support

## Core Components

### gopipe - Pipe Orchestration

The foundation is simple pipe composition:
```go
type Pipe[In, Out any] func(ctx context.Context, in <-chan In) <-chan Out
```

Build robust pipelines with:
- **Concurrency & Batching**: Parallel processing and batching
- **Context-awareness**: Native cancellation and timeout support
- **Error Handling**: Retry, recover, and error propagation
- **Middleware**: Custom logging and metrics
- **Zero Dependencies**: 100% Go standard library

### Channel Package

Utilities for channel operations:
- **Transform**: `Filter`, `Transform`, `Process`
- **Routing**: `Route`, `Merge`, `Broadcast`
- **Batching**: `Collect`, `Buffer`, `GroupBy`
- **Lifecycle**: `Sink`, `Drain`, `Cancel`

See [examples/](examples/) for usage patterns.

### Message Package

Complete message handling system:
- **Pub/Sub**: Publisher/Subscriber with multiple broker implementations
- **Routing**: Attribute-based message dispatch with handlers
- **CQRS**: Type-safe command and event handlers
- **CloudEvents**: CloudEvents v1.0.2 HTTP Protocol Binding support
- **Middleware**: Correlation IDs, logging, and custom middleware

**Broker Implementations:**
- `broker.NewChannelBroker()` - In-memory for testing
- `broker.NewHTTPSender/Receiver()` - HTTP webhooks with CloudEvents
- `broker.NewIOBroker()` - Debug/logging broker (JSONL)

**Advanced Features:**
- [Message Router](docs/features/04-message-router.md) - Attribute-based routing
- [CQRS Handlers](docs/features/05-message-cqrs.md) - Command/event patterns
- [CloudEvents](docs/features/06-message-cloudevents.md) - CloudEvents support
- [Multiplex](docs/features/07-message-multiplex.md) - Topic-based routing
- [Middleware](docs/features/08-middleware-package.md) - Reusable middleware

## Why gopipe?

Manual channel wiring is error-prone and doesn't scale. gopipe provides:
- **Type-safe pipelines**: Generic API works with any data type
- **Battle-tested patterns**: Pub/sub, CQRS, event sourcing
- **Production-ready**: Comprehensive testing and CloudEvents compliance
- **Zero dependencies**: Pure Go, no external dependencies

## Full Feature List of Pipe Options

- `WithConcurrency`: Optional concurrency for parallel processing.
- `WithCancel`: Optional cancellation logic.
- `WithBuffer`: Optional buffered output channel.
- `WithTimeout`: Optional processing timeout via context.
- `WithoutContextPropagation`: Opt-out for propagating the parent context to the processing context to prevent cancellation.
- `WithLogConfig`: Customizable logging - defaults to success (debug), cancel (warn) and failure (error) with `log/slog`.
- `WithMetricsCollector`: Optional processing metrics can be retrieved and evaluated individually.
- `WithMetadataProvider`: Optional metadata enrichment for log messages and metrics based on input values.
- `WithMiddleware`: Optional support for custom middleware.
- `WithRecover`: Optional recovery on panics.
- `WithRetryConfig`: Optional retry on failure with custom configuration.

## Installation

```bash
go get github.com/fxsml/gopipe
```

## Getting Started

The main concepts are:

- `Processor`: Implements the logic for processing and cancellation.
- `Pipe`: Responsible for configuration and orchestration of a pipeline running a specific `Processor`.

For simple channel operations, see the `channel` package.  
For robust orchestration, use `gopipe`.

## Usage

### Basic Channel Operations: Filter, Transform, Buffer, Sink

```go
package main

import (
	"fmt"

	"github.com/fxsml/gopipe/channel"
)

func main() {
	// Create an input channel
	in := channel.FromRange(10)

	// Filter even numbers only
	filtered := channel.Filter(in, func(i int) bool {
		return i%2 == 0
	})

	// Transform values (int -> string)
	transformed := channel.Transform(filtered, func(i int) string {
		return fmt.Sprintf("Value: %d", i)
	})

	// Add buffering
	buffered := channel.Buffer(transformed, 10)

	// Consume values and wait for completion
	<-channel.Sink(buffered, func(s string) {
		fmt.Println(s)
	})
}
```

### Advanced Channel Operations: Merge, Flatten, Process, Route

```go
package main

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipe/channel"
)

type Article struct {
	ID   string
	Name string
	Shop string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create input with two single articles
	ch1 := channel.FromSlice([]Article{
		{ID: "CH1.1", Name: "Laptop"},
		{ID: "CH1.2", Name: "Phone"},
	})

	// Create input with one slice of articles
	ch2 := channel.FromValues([]Article{
		{ID: "CH3.1", Name: "Tablet"},
		{ID: "CH3.2", Name: "Watch"},
		{ID: "CH3.3", Name: "Sensor"},
	})

	// Merge article channels and flatten slices from ch2
	articlesCh := channel.Merge(ch1, channel.Flatten(ch2))

	// Create a list of shops
	shops := []string{"ShopA", "ShopB"}

	// Add cancellation handling before further processing
	// to stop processing on context cancellation
	articlesCh = channel.Cancel(ctx, articlesCh, func(a Article, err error) {
		fmt.Printf("Processing article %s canceled: %v\n", a.ID, err)
	})

	// Expand articles to multiple shops
	articlesCh = channel.Process(articlesCh, func(a Article) []Article {
		articles := make([]Article, len(shops))
		for i, shop := range shops {
			articles[i] = Article{
				ID:   a.ID,
				Name: a.Name,
				Shop: shop,
			}
		}
		return articles
	})

	// Route shop articles based on shop name
	routed := channel.Route(articlesCh, func(a Article) int {
		switch a.Shop {
		case "ShopA":
			return 0
		case "ShopB":
			return 1
		default:
			return -1
		}
	}, len(shops))

	// Create sinks for each shop
	doneChans := make([]<-chan struct{}, len(shops))
	for i, r := range routed {
		doneChans[i] = channel.Sink(r, func(a Article) {
			fmt.Printf("%s: %s (%s)\n", a.Shop, a.Name, a.ID)
		})
	}

	// Wait for all sinks to complete
	<-channel.Merge(doneChans...)
}
```

### Transform with Pipe

```go
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
```

### Batch with Pipe

```go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
)

// User is a simple user struct for demonstration
type User struct {
	ID   int
	Name string
}

// UserResponse encapsulates the result of a user creation attempt
type UserResponse struct {
	User User
	Err  error
}

// NewCreateUserHandler simulates creating users in batches (e.g. database inserts).
func NewCreateUserHandler() func(context.Context, []string) ([]UserResponse, error) {
	currentID := 1000
	return func(ctx context.Context, names []string) ([]UserResponse, error) {
		// Simulate an error causing the whole batch to fail
		if currentID%3 == 0 {
			defer func() {
				currentID++
			}()
			return nil, fmt.Errorf("create user id '%d'", currentID)
		}

		users := make([]UserResponse, 0, len(names))
		for _, name := range names {
			currentID++
			// Simulate an error for individual name
			if strings.ContainsAny(name, "!@#$%^&*()+=[]{}|\\;:'\",.<>/?`~") {
				users = append(users, UserResponse{Err: fmt.Errorf("invalid name: %q", name)})
				continue
			}
			u := User{Name: name, ID: currentID}
			users = append(users, UserResponse{User: u})
		}
		return users, nil
	}
}

func main() {
	// Create an input channel
	in := make(chan string, 10)

	// Start a goroutine to send new user names - for simplicity just runes
	go func() {
		defer func() {
			close(in)
		}()
		for _, c := range "a+bcdefgh!ijkl?mn@op>qrs#tuvwxyz" {
			in <- string(c)
		}
	}()

	// Create a pipe
	pipe := gopipe.NewBatchPipe(
		NewCreateUserHandler(),
		5,                   // Max batch size
		10*time.Millisecond, // Max batch duration
		gopipe.WithBuffer[[]string, UserResponse](10), // Buffer up to 10 results
	)

	// Create new users in batches
	userResponses := pipe.Start(context.Background(), in)

	// Consume responses
	<-channel.Sink(userResponses, func(userResponse UserResponse) {
		if userResponse.Err != nil {
			fmt.Printf("Failed to create new user: %v\n", userResponse.Err)
			return
		}
		fmt.Printf("Created new user: %v\n", userResponse.User)
	})
}
```

### Message Pub/Sub with CloudEvents

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func main() {
	ctx := context.Background()

	// Create in-memory broker
	brk := broker.NewChannelBroker()

	// Setup publisher with batching
	pub := message.NewPublisher(brk, message.PublisherConfig{
		MaxBatchSize: 100,
		MaxDuration:  time.Second,
	})

	// Setup subscriber
	sub := message.NewSubscriber(brk, message.SubscriberConfig{
		PollInterval: 100 * time.Millisecond,
	})

	// Subscribe to topic
	msgs := sub.Subscribe(ctx, "orders")

	// Publish messages
	pub.Publish(ctx, []*message.Message{
		{
			Data: []byte(`{"orderId": "123", "amount": 100}`),
			Attributes: message.Attributes{
				message.AttrID:      "evt-1",
				message.AttrType:    "order.created",
				message.AttrSubject: "orders",
			},
		},
	})

	// Consume messages
	timeout := time.After(2 * time.Second)
	select {
	case msg := <-msgs:
		fmt.Printf("Received: %s (type: %s)\n",
			string(msg.Data),
			msg.Attributes[message.AttrType])
	case <-timeout:
		fmt.Println("No messages received")
	}
}
```

See [docs/features/03-message-pubsub.md](docs/features/03-message-pubsub.md) for more pub/sub patterns.

### Message Acknowledgment for Reliable Processing

```go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

func main() {
	ctx := context.Background()

	// Simulate message broker integration with ack/nack callbacks
	ack := func() { fmt.Println("✓ Message acknowledged") }
	nack := func(err error) { fmt.Printf("✗ Message rejected: %v\n", err) }

	// Create messages using the new functional options API
	in := channel.FromValues(
		message.New(12,
			message.WithContext[int](ctx),
			message.WithAcking[int](ack, nack),
			message.WithID[int]("msg-001"),
			message.WithProperty[int]("source", "orders-queue"),
		),
		message.New(42,
			message.WithContext[int](ctx),
			message.WithAcking[int](ack, nack),
			message.WithID[int]("msg-002"),
			message.WithProperty[int]("source", "orders-queue"),
		),
	)

	// Create pipe with acknowledgment
	pipe := gopipe.NewTransformPipe(
		func(ctx context.Context, msg *message.Message[int]) (*message.Message[int], error) {
			defer msg.Properties().Set("processed_at", time.Now().Format(time.RFC3339))

			// Simulate processing error
			p := msg.Payload()
			if p == 12 {
				err := fmt.Errorf("cannot process payload 12")
				msg.Nack(err)
				return nil, err
			}

			// On success
			res := p * 2
			msg.Ack()
			return message.Copy(msg, res), nil
		},
	)

	// Process message
	results := pipe.Start(ctx, in)

	// Consume results
	<-channel.Sink(results, func(result *message.Message[int]) {
		var sb strings.Builder
		result.Properties().Range(func(key string, value any) bool {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
			return true
		})
		fmt.Printf("Payload: %d\nProperties:\n%s", result.Payload(), sb.String())
	})
}
```

