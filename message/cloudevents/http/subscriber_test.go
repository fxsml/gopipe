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

func TestSubscriber_CloseScenarios(t *testing.T) {
	t.Run("close before request returns 503", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		sub.Close()

		body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("close during send returns 503", func(t *testing.T) {
		// Use buffer size 0 so send blocks
		sub := NewSubscriber(SubscriberConfig{BufferSize: 1})

		// Fill the buffer so next send blocks
		go func() {
			body := []byte(`{"specversion":"1.0","id":"filler","type":"test","source":"/test","data":{}}`)
			req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
			w := httptest.NewRecorder()
			sub.ServeHTTP(w, req) // This will block on send
		}()

		// Wait a bit for goroutine to start and block
		time.Sleep(50 * time.Millisecond)

		// Now send another request that will block
		done := make(chan int)
		go func() {
			body := []byte(`{"specversion":"1.0","id":"blocked","type":"test","source":"/test","data":{}}`)
			req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
			w := httptest.NewRecorder()
			sub.ServeHTTP(w, req)
			done <- w.Code
		}()

		// Wait for request to block on send
		time.Sleep(50 * time.Millisecond)

		// Close the subscriber - this should unblock the request
		go sub.Close()

		select {
		case code := <-done:
			if code != http.StatusServiceUnavailable {
				t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete after Close()")
		}
	})

	t.Run("close during ack wait returns 503", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: 10 * time.Second})

		done := make(chan int)
		go func() {
			body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
			req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
			w := httptest.NewRecorder()
			sub.ServeHTTP(w, req) // Will block waiting for ack
			done <- w.Code
		}()

		// Wait for message to be received but don't ack
		msg := <-sub.C()
		_ = msg // Don't ack

		// Close the subscriber
		go sub.Close()

		select {
		case code := <-done:
			if code != http.StatusServiceUnavailable {
				t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete after Close()")
		}
	})

	t.Run("close waits for in-flight requests", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})

		requestStarted := make(chan struct{})
		requestDone := make(chan struct{})

		go func() {
			body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
			req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
			w := httptest.NewRecorder()
			close(requestStarted)
			sub.ServeHTTP(w, req)
			close(requestDone)
		}()

		<-requestStarted
		time.Sleep(10 * time.Millisecond) // Let request start processing

		// Consume and ack in background after a delay
		go func() {
			msg := <-sub.C()
			time.Sleep(50 * time.Millisecond) // Simulate processing
			msg.Ack()
		}()

		// Close should wait for the request to complete
		closeDone := make(chan struct{})
		go func() {
			sub.Close()
			close(closeDone)
		}()

		// Request should complete before Close returns
		select {
		case <-requestDone:
			// Good - request completed
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete")
		}

		select {
		case <-closeDone:
			// Good - Close returned after request completed
		case <-time.After(2 * time.Second):
			t.Fatal("Close() did not return")
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
