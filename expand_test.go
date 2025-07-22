package gopipe

import (
	"testing"
)

func TestExpand_BasicMapping(t *testing.T) {
	in := make(chan int)
	out := Expand(in, func(v int) []int {
		return []int{v, v * 10}
	})

	go func() {
		in <- 1
		in <- 2
		close(in)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	expected := []int{1, 10, 2, 20}
	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("at %d expected %d got %d", i, expected[i], got[i])
		}
	}
}

func TestExpand_EmptyAndNil(t *testing.T) {
	in := make(chan int)
	out := Expand(in, func(v int) []int {
		if v == 0 {
			return nil
		}
		if v == 1 {
			return []int{}
		}
		return []int{v}
	})

	go func() {
		in <- 0
		in <- 1
		in <- 2
		close(in)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	expected := []int{2}
	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	if got[0] != 2 {
		t.Fatalf("expected 2, got %d", got[0])
	}
}
