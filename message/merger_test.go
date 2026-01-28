package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fxsml/gopipe/pipe"
)

func TestMerger_BasicMerge(t *testing.T) {
	merger := NewMerger(MergerConfig{})

	in1 := make(chan *Message, 2)
	in2 := make(chan *Message, 2)

	in1 <- &Message{Data: "a", Attributes: Attributes{"source": "1"}}
	in1 <- &Message{Data: "b", Attributes: Attributes{"source": "1"}}
	close(in1)

	in2 <- &Message{Data: "c", Attributes: Attributes{"source": "2"}}
	in2 <- &Message{Data: "d", Attributes: Attributes{"source": "2"}}
	close(in2)

	_, _ = merger.AddInput(in1)
	_, _ = merger.AddInput(in2)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out, err := merger.Merge(ctx)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	var received int
	for range out {
		received++
	}

	if received != 4 {
		t.Errorf("expected 4 messages, got %d", received)
	}
}

func TestMerger_AutoNackOnShutdown(t *testing.T) {
	nackCh := make(chan error, 100)

	merger := NewMerger(MergerConfig{
		BufferSize:      1, // Small buffer to cause blocking
		ShutdownTimeout: 50 * time.Millisecond,
		ErrorHandler: func(msg *Message, err error) {
			nackCh <- err
		},
	})

	acking := NewAcking(func() {}, func(err error) {
		nackCh <- err
	})

	in := make(chan *Message, 10)
	// Send multiple messages - some will be dropped on shutdown
	for i := 0; i < 10; i++ {
		in <- &Message{
			Data:       i,
			Attributes: Attributes{"type": "test"},
			acking:     acking,
		}
	}
	// Don't close input - will trigger shutdown timeout

	_, _ = merger.AddInput(in)

	ctx, cancel := context.WithCancel(context.Background())
	out, _ := merger.Merge(ctx)

	// Read one message to start the merger
	<-out

	// Cancel while messages still in input
	cancel()

	// Wait for a nack error (from shutdown timeout dropping messages)
	select {
	case err := <-nackCh:
		if !errors.Is(err, pipe.ErrShutdownDropped) {
			t.Errorf("expected ErrShutdownDropped, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected message to be nacked on shutdown timeout")
	}

	// Drain remaining output
	for range out {
	}
}

func TestMerger_ConfigDefaults(t *testing.T) {
	cfg := MergerConfig{}.parse()

	if cfg.BufferSize != 100 {
		t.Errorf("expected default BufferSize=100, got %d", cfg.BufferSize)
	}
	if cfg.Logger == nil {
		t.Error("expected default Logger")
	}
}
