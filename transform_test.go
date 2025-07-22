package gopipe

import "testing"

func TestTransform_Basic(t *testing.T) {
    in := make(chan int)
    out := Transform(in, func(v int) int { return v * 2 })

    go func() {
        in <- 1
        in <- 2
        in <- 3
        close(in)
    }()

    var got []int
    for v := range out {
        got = append(got, v)
    }

    expected := []int{2, 4, 6}
    if len(got) != len(expected) {
        t.Fatalf("expected %v, got %v", expected, got)
    }
    for i := range expected {
        if got[i] != expected[i] {
            t.Fatalf("at %d expected %d got %d", i, expected[i], got[i])
        }
    }
}
