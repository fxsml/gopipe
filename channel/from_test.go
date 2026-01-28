package channel

import (
	"context"
	"reflect"
	"testing"
)

func TestFromSlice(t *testing.T) {
	cases := []struct {
		name   string
		input  []int
		expect []int
	}{
		{"empty slice", []int{}, []int{}},
		{"single element", []int{42}, []int{42}},
		{"multiple elements", []int{1, 2, 3}, []int{1, 2, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ch := FromSlice(c.input)
			var result []int
			for v := range ch {
				result = append(result, v)
			}
			if len(c.expect) == 0 && len(result) == 0 {
				return
			}
			if !reflect.DeepEqual(result, c.expect) {
				t.Errorf("expected %v, got %v", c.expect, result)
			}
		})
	}
}

func TestFromRange(t *testing.T) {
	cases := []struct {
		name   string
		args   []int
		expect []int
	}{
		{"empty range", []int{0}, []int{}},
		{"single value", []int{5, 6, 1}, []int{5}},
		{"step 1", []int{0, 3, 1}, []int{0, 1, 2}},
		{"step 2", []int{0, 5, 2}, []int{0, 2, 4}},
		{"default from/step", []int{3}, []int{0, 1, 2}},
		{"from/to only", []int{2, 5}, []int{2, 3, 4}},
		{"negative step", []int{5, 2, -1}, []int{5, 4, 3}},
		{"step larger than range", []int{0, 5, 10}, []int{0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ch <-chan int
			switch len(c.args) {
			case 1:
				ch = FromRange(c.args[0])
			case 2:
				ch = FromRange(c.args[0], c.args[1])
			case 3:
				ch = FromRange(c.args[0], c.args[1], c.args[2])
			}
			var result []int
			for v := range ch {
				result = append(result, v)
			}
			if len(c.expect) == 0 && len(result) == 0 {
				return
			}
			if !reflect.DeepEqual(result, c.expect) {
				t.Errorf("expected %v, got %v", c.expect, result)
			}
		})
	}
}

func TestFromFunc(t *testing.T) {
	// Example: generate values until cancelled
	ctx, cancel := context.WithCancel(context.Background())

	i := 0
	handle := func() int {
		val := i
		i++
		return val
	}

	ch := FromFunc(ctx, handle)
	var result []int
	for v := range ch {
		result = append(result, v)
		if len(result) >= 3 {
			cancel() // Cancel after receiving 3 values
		}
	}
	// Should receive at least 0, 1, 2 (may receive more due to race)
	if len(result) < 3 || result[0] != 0 || result[1] != 1 || result[2] != 2 {
		t.Errorf("expected at least [0 1 2], got %v", result)
	}

	// Test: handle returns false immediately
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	handle2 := func() int { return 0 }
	ch2 := FromFunc(ctx, handle2)
	var result2 []int
	for v := range ch2 {
		result2 = append(result2, v)
	}
	if len(result2) != 0 {
		t.Errorf("expected empty, got %v", result2)
	}
}
