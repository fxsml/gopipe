package http

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
)

func TestHTTP_E2E_SingleEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start subscriber
	sub := NewSubscriber(SubscriberConfig{
		Addr:       "127.0.0.1:0",
		Path:       "/events",
		BufferSize: 10,
	})

	ordersCh, err := sub.Subscribe(ctx, "orders")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- sub.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)
	addr := sub.Addr()
	if addr == nil {
		t.Fatal("server did not start")
	}

	// Create publisher
	pub := NewPublisher(PublisherConfig{
		TargetURL: fmt.Sprintf("http://%s/events", addr.String()),
		Client:    &http.Client{Timeout: 5 * time.Second},
	})

	// Receive messages in background
	received := make(chan *message.RawMessage, 1)
	go func() {
		for msg := range ordersCh {
			received <- msg
			msg.Ack()
		}
	}()

	// Send event
	msg := message.NewRaw(
		[]byte(`{"order_id":"ORD-001"}`),
		message.Attributes{
			message.AttrID:     "test-1",
			message.AttrType:   "order.created",
			message.AttrSource: "/test",
		},
		nil,
	)

	err = pub.Publish(ctx, "orders", msg)
	if err != nil {
		t.Fatalf("publish error: %v", err)
	}

	// Verify receipt
	select {
	case r := <-received:
		if r.ID() != "test-1" {
			t.Errorf("expected id 'test-1', got %v", r.ID())
		}
		if r.Type() != "order.created" {
			t.Errorf("expected type 'order.created', got %v", r.Type())
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for message")
	}
}

func TestHTTP_E2E_BatchEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start subscriber
	sub := NewSubscriber(SubscriberConfig{
		Addr:       "127.0.0.1:0",
		Path:       "/events",
		BufferSize: 100,
	})

	ordersCh, err := sub.Subscribe(ctx, "orders")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	go sub.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Create publisher
	pub := NewPublisher(PublisherConfig{
		TargetURL: fmt.Sprintf("http://%s/events", sub.Addr().String()),
	})

	// Receive messages
	var receivedCount atomic.Int32
	go func() {
		for msg := range ordersCh {
			receivedCount.Add(1)
			msg.Ack()
		}
	}()

	// Send batch via PublishBatch
	inputCh := make(chan *message.RawMessage, 100)
	done, err := pub.PublishBatch(ctx, "orders", inputCh, BatchConfig{
		MaxSize:     5,
		MaxDuration: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("batch publish error: %v", err)
	}

	// Send 10 messages
	for i := 0; i < 10; i++ {
		inputCh <- message.NewRaw(
			[]byte(fmt.Sprintf(`{"order_id":"ORD-%03d"}`, i)),
			message.Attributes{
				message.AttrID:     fmt.Sprintf("batch-%d", i),
				message.AttrType:   "order.created",
				message.AttrSource: "/test",
			},
			nil,
		)
	}
	close(inputCh)

	// Wait for batch publishing to complete
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for batch publish")
	}

	// Give subscriber time to process
	time.Sleep(200 * time.Millisecond)

	if receivedCount.Load() != 10 {
		t.Errorf("expected 10 messages, got %d", receivedCount.Load())
	}
}

func TestHTTP_E2E_MultiTopic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sub := NewSubscriber(SubscriberConfig{
		Addr:       "127.0.0.1:0",
		Path:       "/events",
		BufferSize: 10,
	})

	ordersCh, _ := sub.Subscribe(ctx, "orders")
	paymentsCh, _ := sub.Subscribe(ctx, "payments")

	go sub.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	pub := NewPublisher(PublisherConfig{
		TargetURL: fmt.Sprintf("http://%s/events", sub.Addr().String()),
	})

	// Receive from both topics
	var ordersCount, paymentsCount atomic.Int32
	go func() {
		for msg := range ordersCh {
			ordersCount.Add(1)
			msg.Ack()
		}
	}()
	go func() {
		for msg := range paymentsCh {
			paymentsCount.Add(1)
			msg.Ack()
		}
	}()

	// Send to different topics
	for i := 0; i < 3; i++ {
		pub.Publish(ctx, "orders", message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     fmt.Sprintf("o%d", i),
			message.AttrType:   "order",
			message.AttrSource: "/test",
		}, nil))
	}

	for i := 0; i < 2; i++ {
		pub.Publish(ctx, "payments", message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     fmt.Sprintf("p%d", i),
			message.AttrType:   "payment",
			message.AttrSource: "/test",
		}, nil))
	}

	time.Sleep(200 * time.Millisecond)

	if ordersCount.Load() != 3 {
		t.Errorf("expected 3 orders, got %d", ordersCount.Load())
	}
	if paymentsCount.Load() != 2 {
		t.Errorf("expected 2 payments, got %d", paymentsCount.Load())
	}
}

func TestHTTP_E2E_Stream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sub := NewSubscriber(SubscriberConfig{
		Addr:       "127.0.0.1:0",
		Path:       "/events",
		BufferSize: 100,
	})

	ch, _ := sub.Subscribe(ctx, "stream")
	go sub.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	pub := NewPublisher(PublisherConfig{
		TargetURL:   fmt.Sprintf("http://%s/events", sub.Addr().String()),
		Concurrency: 4,
	})

	var receivedCount atomic.Int32
	go func() {
		for msg := range ch {
			receivedCount.Add(1)
			msg.Ack()
		}
	}()

	inputCh := make(chan *message.RawMessage, 100)
	done, err := pub.PublishStream(ctx, "stream", inputCh)
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	// Stream 20 messages
	for i := 0; i < 20; i++ {
		inputCh <- message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     fmt.Sprintf("s%d", i),
			message.AttrType:   "stream",
			message.AttrSource: "/test",
		}, nil)
	}
	close(inputCh)

	<-done
	time.Sleep(200 * time.Millisecond)

	if receivedCount.Load() != 20 {
		t.Errorf("expected 20 messages, got %d", receivedCount.Load())
	}
}
