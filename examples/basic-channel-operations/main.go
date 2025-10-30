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

	// Output:
	// Value: 0
	// Value: 2
	// Value: 4
	// Value: 6
	// Value: 8
}
