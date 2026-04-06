package redis

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/alicebob/miniredis/v2"
	"github.com/fxsml/gopipe/message"
)

func TestSubscriberPublisher_EndToEnd(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Subscribe to input stream
	sub := NewSubscriber(client, SubscriberConfig{
		Stream:       "input-stream",
		Group:        "processor",
		Consumer:     "worker-1",
		BlockTimeout: 100 * time.Millisecond,
		BufferSize:   10,
	})

	// Publish to output stream
	pub := NewPublisher(client, PublisherConfig{
		Stream: "output-stream",
	})

	// Start subscriber
	inputCh, err := sub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Process: read from subscriber, transform, send to publisher
	outputCh := make(chan *message.RawMessage, 10)
	pubDone, err := pub.Publish(ctx, outputCh)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Produce messages to input stream
	for i := range 5 {
		values, _ := DefaultMarshal(message.NewRaw(
			[]byte(`{"value":`+string(rune('0'+i))+`}`),
			message.Attributes{
				message.AttrID:   "e2e-" + string(rune('0'+i)),
				message.AttrType: "input.event",
			},
			nil,
		))
		client.XAdd(ctx, &goredis.XAddArgs{Stream: "input-stream", Values: values})
	}

	// Process messages: subscriber → transform → publisher
	processed := 0
	for processed < 5 {
		select {
		case msg := <-inputCh:
			if msg == nil {
				t.Fatal("channel closed unexpectedly")
			}

			// Transform: create output message with different type
			outAcked := make(chan struct{})
			outAcking := message.NewAcking(
				func() {
					close(outAcked)
					msg.Ack() // ack input after output is confirmed
				},
				func(err error) { msg.Nack(err) },
			)

			out := message.NewRaw(
				msg.Data,
				message.Attributes{
					message.AttrID:   msg.ID() + "-out",
					message.AttrType: "output.event",
				},
				outAcking,
			)

			outputCh <- out
			<-outAcked
			processed++
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout after processing %d/5 messages", processed)
		}
	}

	close(outputCh)
	<-pubDone

	// Verify: input stream has 5 messages, all acked (empty PEL)
	pending, _ := client.XPending(ctx, "input-stream", "processor").Result()
	if pending.Count != 0 {
		t.Fatalf("expected empty input PEL, got %d", pending.Count)
	}

	// Verify: output stream has 5 messages
	outputMsgs, err := client.XRange(ctx, "output-stream", "-", "+").Result()
	if err != nil {
		t.Fatalf("xrange: %v", err)
	}
	if len(outputMsgs) != 5 {
		t.Fatalf("expected 5 output messages, got %d", len(outputMsgs))
	}
}
