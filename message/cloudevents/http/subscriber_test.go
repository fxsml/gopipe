package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubscriber_Subscribe(t *testing.T) {
	t.Run("returns channel for topic", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ch == nil {
			t.Fatal("expected non-nil channel")
		}
	})

	t.Run("returns error for duplicate topic", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = sub.Subscribe(ctx, "orders")
		if err == nil {
			t.Fatal("expected error for duplicate topic")
		}
	})

	t.Run("allows multiple different topics", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch1, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ch2, err := sub.Subscribe(ctx, "payments")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ch1 == ch2 {
			t.Fatal("expected different channels for different topics")
		}
	})

	t.Run("closes channel on context cancellation", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())

		ch, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cancel()

		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected channel to be closed")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for channel close")
		}
	})
}

func TestSubscriber_ServeHTTP(t *testing.T) {
	t.Run("rejects non-POST", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{Path: "/events"})

		req := httptest.NewRequest(http.MethodGet, "/events/orders", nil)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("returns 404 for unknown topic", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{Path: "/events"})

		body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
		req := httptest.NewRequest(http.MethodPost, "/events/unknown", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("parses single event and delivers to channel", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{Path: "/events", AckTimeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("subscribe error: %v", err)
		}

		// Consume message and ack in background
		go func() {
			msg := <-ch
			if msg.ID() != "test-1" {
				t.Errorf("expected id 'test-1', got %v", msg.ID())
			}
			msg.Ack()
		}()

		body := []byte(`{"specversion":"1.0","id":"test-1","type":"order.created","source":"/shop","data":{"order_id":"123"}}`)
		req := httptest.NewRequest(http.MethodPost, "/events/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("returns 500 on nack", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{Path: "/events", AckTimeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("subscribe error: %v", err)
		}

		// Nack the message
		go func() {
			msg := <-ch
			msg.Nack(errors.New("processing failed"))
		}()

		body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
		req := httptest.NewRequest(http.MethodPost, "/events/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	t.Run("returns 400 on invalid JSON", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{Path: "/events"})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("subscribe error: %v", err)
		}

		body := []byte(`not valid json`)
		req := httptest.NewRequest(http.MethodPost, "/events/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("parses batch events", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{Path: "/events", AckTimeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := sub.Subscribe(ctx, "orders")
		if err != nil {
			t.Fatalf("subscribe error: %v", err)
		}

		// Consume and ack all messages
		go func() {
			for i := 0; i < 2; i++ {
				msg := <-ch
				msg.Ack()
			}
		}()

		body := []byte(`[
			{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}},
			{"specversion":"1.0","id":"2","type":"test","source":"/test","data":{}}
		]`)
		req := httptest.NewRequest(http.MethodPost, "/events/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsBatchJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})
}

func TestSubscriber_Handler(t *testing.T) {
	sub := NewSubscriber(SubscriberConfig{Path: "/events", AckTimeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := sub.Subscribe(ctx, "orders")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	handler := sub.Handler("orders")

	go func() {
		msg := <-ch
		msg.Ack()
	}()

	body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/any/path", bytes.NewReader(body))
	req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, w.Code)
	}
}
