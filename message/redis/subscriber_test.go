package redis

import (
	"context"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/alicebob/miniredis/v2"
	"github.com/fxsml/gopipe/message"
)

func TestSubscriber_ReceivesMessages(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub := NewSubscriber(client, SubscriberConfig{
		Stream:       "test-stream",
		Group:        "test-group",
		Consumer:     "test-consumer",
		BatchSize:    10,
		BlockTimeout: 100 * time.Millisecond,
		BufferSize:   10,
	})

	// Publish messages before subscribing
	for i := range 3 {
		data := []byte(`{"i":` + string(rune('0'+i)) + `}`)
		attrs := message.Attributes{message.AttrID: "msg-" + string(rune('0'+i))}
		values, _ := DefaultMarshal(message.NewRaw(data, attrs, nil))
		client.XAdd(ctx, &goredis.XAddArgs{
			Stream: "test-stream",
			Values: values,
		})
	}

	ch, err := sub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	var received []*message.RawMessage
	timeout := time.After(2 * time.Second)
	for len(received) < 3 {
		select {
		case msg := <-ch:
			if msg == nil {
				t.Fatal("channel closed unexpectedly")
			}
			received = append(received, msg)
			msg.Ack()
		case <-timeout:
			t.Fatalf("timeout waiting for messages, got %d/3", len(received))
		}
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(received))
	}
}

func TestSubscriber_AckRemovesFromPEL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub := NewSubscriber(client, SubscriberConfig{
		Stream:       "ack-stream",
		Group:        "ack-group",
		Consumer:     "ack-consumer",
		BlockTimeout: 100 * time.Millisecond,
		BufferSize:   10,
	})

	// Publish one message
	values, _ := DefaultMarshal(message.NewRaw([]byte(`{}`), message.Attributes{message.AttrID: "ack-1"}, nil))
	client.XAdd(ctx, &goredis.XAddArgs{Stream: "ack-stream", Values: values})

	ch, err := sub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case msg := <-ch:
		// Before ack: message should be in PEL
		pending, _ := client.XPending(ctx, "ack-stream", "ack-group").Result()
		if pending.Count == 0 {
			t.Fatal("expected message in PEL before ack")
		}

		msg.Ack()
		// Give XACK time to execute
		time.Sleep(50 * time.Millisecond)

		// After ack: PEL should be empty
		pending, _ = client.XPending(ctx, "ack-stream", "ack-group").Result()
		if pending.Count != 0 {
			t.Fatalf("expected empty PEL after ack, got %d", pending.Count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_NackLeaveInPEL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub := NewSubscriber(client, SubscriberConfig{
		Stream:       "nack-stream",
		Group:        "nack-group",
		Consumer:     "nack-consumer",
		BlockTimeout: 100 * time.Millisecond,
		BufferSize:   10,
	})

	// Publish one message
	values, _ := DefaultMarshal(message.NewRaw([]byte(`{}`), message.Attributes{message.AttrID: "nack-1"}, nil))
	client.XAdd(ctx, &goredis.XAddArgs{Stream: "nack-stream", Values: values})

	ch, err := sub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case msg := <-ch:
		msg.Nack(nil)
		time.Sleep(50 * time.Millisecond)

		// After nack: message should still be in PEL
		pending, _ := client.XPending(ctx, "nack-stream", "nack-group").Result()
		if pending.Count == 0 {
			t.Fatal("expected message to stay in PEL after nack")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_AlreadyStarted(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := NewSubscriber(client, SubscriberConfig{
		Stream:       "started-stream",
		Group:        "started-group",
		BlockTimeout: 100 * time.Millisecond,
	})

	_, err := sub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("first subscribe: %v", err)
	}

	_, err = sub.Subscribe(ctx)
	if err == nil {
		t.Fatal("expected error on second subscribe")
	}
}

func TestSubscriber_ConcurrentAcks(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub := NewSubscriber(client, SubscriberConfig{
		Stream:       "concurrent-stream",
		Group:        "concurrent-group",
		Consumer:     "concurrent-consumer",
		BatchSize:    20,
		BlockTimeout: 100 * time.Millisecond,
		BufferSize:   20,
	})

	// Publish 10 messages
	for i := range 10 {
		values, _ := DefaultMarshal(message.NewRaw(
			[]byte(`{}`),
			message.Attributes{message.AttrID: "conc-" + string(rune('a'+i))},
			nil,
		))
		client.XAdd(ctx, &goredis.XAddArgs{Stream: "concurrent-stream", Values: values})
	}

	ch, err := sub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	var wg sync.WaitGroup
	for range 10 {
		select {
		case msg := <-ch:
			wg.Add(1)
			go func() {
				defer wg.Done()
				msg.Ack()
			}()
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for messages")
		}
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// All messages should be acked
	pending, _ := client.XPending(ctx, "concurrent-stream", "concurrent-group").Result()
	if pending.Count != 0 {
		t.Fatalf("expected empty PEL, got %d pending", pending.Count)
	}
}
