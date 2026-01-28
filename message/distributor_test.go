package message

import (
	"context"
	"testing"
	"time"
)

func TestDistributor_BasicDistribute(t *testing.T) {
	dist := NewDistributor(DistributorConfig{})

	// Output 1: matches type=a
	out1, _ := dist.AddOutput(matcherFunc(func(attrs Attributes) bool {
		return attrs["type"] == "a"
	}))

	// Output 2: matches type=b
	out2, _ := dist.AddOutput(matcherFunc(func(attrs Attributes) bool {
		return attrs["type"] == "b"
	}))

	in := make(chan *Message, 4)
	in <- &Message{Data: "1", Attributes: Attributes{"type": "a"}}
	in <- &Message{Data: "2", Attributes: Attributes{"type": "b"}}
	in <- &Message{Data: "3", Attributes: Attributes{"type": "a"}}
	in <- &Message{Data: "4", Attributes: Attributes{"type": "b"}}
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done, err := dist.Distribute(ctx, in)
	if err != nil {
		t.Fatalf("Distribute failed: %v", err)
	}

	var count1, count2 int
	for {
		select {
		case _, ok := <-out1:
			if ok {
				count1++
			}
		case _, ok := <-out2:
			if ok {
				count2++
			}
		case <-done:
			goto verify
		case <-ctx.Done():
			t.Fatal("timeout")
		}
	}

verify:
	// Drain remaining
	for range out1 {
		count1++
	}
	for range out2 {
		count2++
	}

	if count1 != 2 {
		t.Errorf("expected 2 messages on out1, got %d", count1)
	}
	if count2 != 2 {
		t.Errorf("expected 2 messages on out2, got %d", count2)
	}
}

func TestDistributor_NilMatcher(t *testing.T) {
	dist := NewDistributor(DistributorConfig{})

	// Nil matcher receives all messages
	out, _ := dist.AddOutput(nil)

	in := make(chan *Message, 2)
	in <- &Message{Data: "1", Attributes: Attributes{"type": "a"}}
	in <- &Message{Data: "2", Attributes: Attributes{"type": "b"}}
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, _ = dist.Distribute(ctx, in)

	var count int
	for range out {
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 messages, got %d", count)
	}
}

func TestDistributor_ConfigDefaults(t *testing.T) {
	cfg := DistributorConfig{}.parse()

	if cfg.BufferSize != 100 {
		t.Errorf("expected default BufferSize=100, got %d", cfg.BufferSize)
	}
	if cfg.Logger == nil {
		t.Error("expected default Logger")
	}
}
