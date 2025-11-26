package channel_test

import (
	"testing"

	"github.com/fxsml/gopipe/channel"
)

func TestToSlice_Ints(t *testing.T) {
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	slice := channel.ToSlice(in)

	if len(slice) != 3 {
		t.Errorf("Expected slice length 3, got %d", len(slice))
	}
	if slice[0] != 1 || slice[1] != 2 || slice[2] != 3 {
		t.Errorf("Expected [1 2 3], got %v", slice)
	}
}

func TestToSlice_Empty(t *testing.T) {
	in := make(chan string)
	close(in)

	slice := channel.ToSlice(in)

	if len(slice) != 0 {
		t.Errorf("Expected empty slice, got %v", slice)
	}
}

func TestToSlice_Strings(t *testing.T) {
	in := make(chan string, 2)
	in <- "foo"
	in <- "bar"
	close(in)

	slice := channel.ToSlice(in)

	if len(slice) != 2 {
		t.Errorf("Expected slice length 2, got %d", len(slice))
	}
	if slice[0] != "foo" || slice[1] != "bar" {
		t.Errorf("Expected [foo bar], got %v", slice)
	}
}
