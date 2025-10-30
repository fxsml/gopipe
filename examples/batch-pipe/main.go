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
