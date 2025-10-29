package channel

import (
	"testing"
)

func TestMerge_AllValues(t *testing.T) {
	a := make(chan int)
	b := make(chan int)
	out := Merge(a, b)

	go func() {
		defer close(a)
		a <- 1
		a <- 2
	}()
	go func() {
		defer close(b)
		b <- 3
		b <- 4
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 values, got %v", got)
	}
}

func TestMerge_ClosedInputs(t *testing.T) {
	a := make(chan int)
	close(a)
	out := Merge(a)

	var read bool
	for range out {
		read = true
	}
	if read {
		t.Fatalf("expected no values from closed input")
	}
}

func TestMerge_ZeroInputs(t *testing.T) {
	out := Merge[int]()
	var read bool
	for range out {
		read = true
	}
	if read {
		t.Fatalf("expected no values when merging zero inputs")
	}
}

func TestMerge_OutputReceivesAllValues(t *testing.T) {
	in1 := make(chan int, 2)
	in2 := make(chan int, 2)
	in1 <- 1
	in1 <- 2
	in2 <- 3
	in2 <- 4
	close(in1)
	close(in2)
	out := Merge(in1, in2)
	var got []int
	for v := range out {
		got = append(got, v)
	}
	want := map[int]bool{1: true, 2: true, 3: true, 4: true}
	if len(got) != 4 {
		t.Errorf("expected 4 values, got %d", len(got))
	}
	for _, v := range got {
		if !want[v] {
			t.Errorf("unexpected value %d", v)
		}
		delete(want, v)
	}
	if len(want) != 0 {
		t.Errorf("missing values: %v", want)
	}
}
