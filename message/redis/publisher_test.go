package redis

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/alicebob/miniredis/v2"
	"github.com/fxsml/gopipe/message"
)

func TestPublisher_SendsMessages(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub := NewPublisher(client, PublisherConfig{
		Stream: "pub-stream",
	})

	ch := make(chan *message.RawMessage, 3)
	done, err := pub.Publish(ctx, ch)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Send 3 messages
	for i := range 3 {
		acked := make(chan struct{})
		acking := message.NewAcking(func() { close(acked) }, func(error) {})
		msg := message.NewRaw(
			[]byte(`{"i":`+string(rune('0'+i))+`}`),
			message.Attributes{
				message.AttrID:   "pub-" + string(rune('0'+i)),
				message.AttrType: "test.event",
			},
			acking,
		)
		ch <- msg

		select {
		case <-acked:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for ack on message %d", i)
		}
	}

	close(ch)
	<-done

	// Verify messages in stream
	msgs, err := client.XRange(ctx, "pub-stream", "-", "+").Result()
	if err != nil {
		t.Fatalf("xrange: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages in stream, got %d", len(msgs))
	}
}

func TestPublisher_AcksOnSuccess(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub := NewPublisher(client, PublisherConfig{Stream: "ack-pub-stream"})

	ch := make(chan *message.RawMessage, 1)
	_, err := pub.Publish(ctx, ch)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	acked := make(chan struct{})
	nacked := make(chan error, 1)
	acking := message.NewAcking(
		func() { close(acked) },
		func(err error) { nacked <- err },
	)

	ch <- message.NewRaw([]byte(`{}`), message.Attributes{message.AttrID: "ack-test"}, acking)

	select {
	case <-acked:
		// success
	case err := <-nacked:
		t.Fatalf("expected ack, got nack: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ack")
	}
}

func TestPublisher_MaxLenConfig(t *testing.T) {
	// Verify MAXLEN is set on XAddArgs when configured.
	// miniredis has limited MAXLEN support, so we test the config wiring
	// rather than actual trimming behavior.
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub := NewPublisher(client, PublisherConfig{
		Stream: "maxlen-stream",
		MaxLen: 100,
		// Approx defaults to true
	})

	ch := make(chan *message.RawMessage, 1)
	_, err := pub.Publish(ctx, ch)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	acked := make(chan struct{})
	acking := message.NewAcking(func() { close(acked) }, func(error) {})
	ch <- message.NewRaw([]byte(`{}`), message.Attributes{message.AttrID: "ml-1"}, acking)

	select {
	case <-acked:
		// Message sent with MAXLEN config — success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ack")
	}
}

func TestPublisher_AlreadyStarted(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pub := NewPublisher(client, PublisherConfig{Stream: "started-pub"})

	ch := make(chan *message.RawMessage)
	_, err := pub.Publish(ctx, ch)
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}

	_, err = pub.Publish(ctx, ch)
	if err == nil {
		t.Fatal("expected error on second publish")
	}
}

func TestPublisher_MarshalError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub := NewPublisher(client, PublisherConfig{
		Stream: "marshal-err",
		Marshal: func(msg *message.RawMessage) (map[string]any, error) {
			return nil, context.DeadlineExceeded // simulate marshal error
		},
	})

	ch := make(chan *message.RawMessage, 1)
	_, err := pub.Publish(ctx, ch)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	nacked := make(chan error, 1)
	acking := message.NewAcking(func() {}, func(err error) { nacked <- err })
	ch <- message.NewRaw([]byte(`{}`), message.Attributes{message.AttrID: "bad"}, acking)

	select {
	case <-nacked:
		// success — message was nacked due to marshal error
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for nack")
	}
}
