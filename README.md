# gopipe

A lightweight, generic Go library for building composable data pipelines using channels.

## Features

- **Generic API**: Works with any data type (Go 1.18+).
- **Simple Core Functions**: Basic channel manipulation (`Transform`, `Filter`, `Broadcast`, `Merge`, etc.).
- **Helper Functions**: Buffering and context cancellation support through `Buffer` and `Cancel`/`Break`.
- **Advanced Processing**: Context-aware processing with `Process` and `Batch` functions.

## Philosophy

gopipe provides minimalistic building blocks for channel-based pipelines. By design, the core functions are simple and focused. The two main processing functions (`Process` and `Batch`) are context-aware and highly configurable with options for concurrency, buffering, and context propagation.

The helper functions `Buffer` and `Cancel` are provided when you need buffering or context-aware cancellation, keeping the core functions simple while still offering all needed functionality.

## Installation

```
go get github.com/fxsml/gopipe
```

## Usage

### Basic Operations: Filter, Transform, Buffer, Sink

```go
package main

import (
	"fmt"

	"github.com/fxsml/gopipe"
)

func main() {
	// Create an input channel
	in := make(chan int)

	// Filter even numbers only
	filtered := gopipe.Filter(in, func(i int) bool {
		return i%2 == 0
	})

	// Transform values (int -> string)
	transformed := gopipe.Transform(filtered, func(i int) string {
		return fmt.Sprintf("Value: %d", i)
	})

	// Add buffering
	buffered := gopipe.Buffer(transformed, 10)

	// Start a goroutine to send values
	go func() {
		defer close(in) // Always close input channels when done
		for i := range 10 {
			in <- i
		}
	}()

	// Consume transformed values
	done := gopipe.Sink(buffered, func(s string) {
		fmt.Println(s)
	})

	// Wait for processing to complete
	<-done
}
```

### Advanced Operations: Merge, Split, Expand, Route

```go
package main

import (
	"fmt"

	"github.com/fxsml/gopipe"
)

type Article struct {
	ID   string
	Name string
	Shop string
}

func main() {
	// Create input channels
	ch1 := make(chan Article)
	ch2 := make(chan []Article)

	// Send sample articles
	go func() {
		ch1 <- Article{ID: "CH1.1", Name: "Laptop"}
		ch1 <- Article{ID: "CH1.2", Name: "Phone"}
		close(ch1)

		ch2 <- []Article{
			{ID: "CH3.1", Name: "Tablet"},
			{ID: "CH3.2", Name: "Watch"},
			{ID: "CH3.3", Name: "Sensor"},
		}
		close(ch2)
	}()

	// Merge article channels and split slices from ch2
	articlesCh := gopipe.Merge(ch1, gopipe.Split(ch2))

	// Create a list of shops
	shops := []string{"ShopA", "ShopB"}

	// Expand articles to multiple shops
	articlesCh = gopipe.Expand(articlesCh, func(a Article) []Article {
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
	routed := gopipe.Route(articlesCh, func(a Article) int {
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
		doneChans[i] = gopipe.Sink(r, func(a Article) {
			fmt.Printf("%s: %s (%s)\n", a.Shop, a.Name, a.ID)
		})
	}

	// Wait for all sinks to complete
	<-gopipe.Merge(doneChans...)
}
```

### Context-Aware Processing

```go
package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/fxsml/gopipe"
)

func main() {
	// Create a context with cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create an input channel
	in := make(chan string, 10)

	// Process values with context-awareness and concurrency
	processed := gopipe.Process(
		ctx,
		in,
		func(ctx context.Context, val string) (int, error) {
			// Simulate processing time
			time.Sleep(100 * time.Millisecond)

			// Convert string to int
			return strconv.Atoi(val)
		},
		func(val string, err error) {
			fmt.Printf("Failed to process %q: %v\n", val, err)
		},
		gopipe.WithConcurrency(5), // Use 5 workers
		gopipe.WithBuffer(10),     // Buffer up to 10 results
	)

	// Start a goroutine to send values
	go func() {
		defer func() {
			close(in)
			// Cancel early, to demonstrate context cancellation
			cancel()
		}()
		for i := range 20 {
			if i%3 == 0 {
				// Introduce some invalid input to demonstrate error handling
				in <- fmt.Sprintf("%d - invalid", i)
				continue
			}
			in <- fmt.Sprintf("%d", i)
		}
	}()

	// Consume processed values
	for val := range processed {
		fmt.Printf("Processed: %d\n", val)
	}
}
```

### Batch Processing

```go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
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

	// Create new users in batches
	userResponses := gopipe.Batch(
		context.Background(),
		in,
		NewCreateUserHandler(),
		func(val []string, err error) {
			fmt.Printf("Batch failed: %v, error: %v\n", val, err)
		},
		5,                     // Max batch size
		10*time.Millisecond,   // Max batch duration
		gopipe.WithBuffer(10), // Buffer up to 10 results
	)

	// Consume responses
	for userResponse := range userResponses {
		if userResponse.Err != nil {
			fmt.Printf("Failed to create new user: %v\n", userResponse.Err)
			continue
		}
		fmt.Printf("Created new user: %v\n", userResponse.User)
	}
}
```
