package channel

import "testing"

func TestFlatten_Basic(t *testing.T) {
	in := make(chan []int)
	out := Flatten(in)

	go func() {
		in <- []int{1, 2}
		in <- []int{3}
		close(in)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	expected := []int{1, 2, 3}
	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("at %d expected %d got %d", i, expected[i], got[i])
		}
	}
}

func TestFlatten_EmptyAndNil(t *testing.T) {
	in := make(chan []int)
	out := Flatten(in)

	go func() {
		in <- nil
		in <- []int{}
		in <- []int{4}
		close(in)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	if len(got) != 1 || got[0] != 4 {
		t.Fatalf("expected [4], got %v", got)
	}
}
