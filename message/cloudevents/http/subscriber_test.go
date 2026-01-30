package http

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubscriber_C(t *testing.T) {
	sub := NewSubscriber(SubscriberConfig{BufferSize: 10})

	ch := sub.C()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}

func TestSubscriber_Close(t *testing.T) {
	sub := NewSubscriber(SubscriberConfig{})

	sub.Close()

	select {
	case _, ok := <-sub.C():
		if ok {
			t.Error("expected channel to be closed")
		}
	default:
		t.Error("channel should be closed and readable")
	}
}

func TestSubscriber_ServeHTTP(t *testing.T) {
	t.Run("rejects non-POST", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})

		req := httptest.NewRequest(http.MethodGet, "/events", nil)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("parses single event and delivers to channel", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})

		// Consume message and ack in background
		go func() {
			msg := <-sub.C()
			if msg.ID() != "test-1" {
				t.Errorf("expected id 'test-1', got %v", msg.ID())
			}
			msg.Ack()
		}()

		body := []byte(`{"specversion":"1.0","id":"test-1","type":"order.created","source":"/shop","data":{"order_id":"123"}}`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("returns 500 on nack", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})

		// Nack the message
		go func() {
			msg := <-sub.C()
			msg.Nack(errors.New("processing failed"))
		}()

		body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	t.Run("returns 400 on invalid JSON", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})

		body := []byte(`not valid json`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("parses batch events", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})

		// Consume and ack all messages
		go func() {
			for i := 0; i < 2; i++ {
				msg := <-sub.C()
				msg.Ack()
			}
		}()

		body := []byte(`[
			{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}},
			{"specversion":"1.0","id":"2","type":"test","source":"/test","data":{}}
		]`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsBatchJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})
}

func TestSubscriber_WithServeMux(t *testing.T) {
	orders := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})
	payments := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})

	mux := http.NewServeMux()
	mux.Handle("/events/orders", orders)
	mux.Handle("/events/payments", payments)

	// Consume from both
	go func() {
		msg := <-orders.C()
		msg.Ack()
	}()
	go func() {
		msg := <-payments.C()
		msg.Ack()
	}()

	// Send to orders
	body := []byte(`{"specversion":"1.0","id":"o1","type":"order","source":"/test","data":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/events/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("orders: expected %d, got %d", http.StatusOK, w.Code)
	}

	// Send to payments
	body = []byte(`{"specversion":"1.0","id":"p1","type":"payment","source":"/test","data":{}}`)
	req = httptest.NewRequest(http.MethodPost, "/events/payments", bytes.NewReader(body))
	req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("payments: expected %d, got %d", http.StatusOK, w.Code)
	}
}
