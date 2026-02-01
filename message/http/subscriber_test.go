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
	t.Run("returns channel", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{BufferSize: 10})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := sub.Subscribe(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch == nil {
			t.Fatal("expected non-nil channel")
		}
	})

	t.Run("returns error if called twice", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := sub.Subscribe(ctx)
		if err != nil {
			t.Fatalf("first subscribe failed: %v", err)
		}

		_, err = sub.Subscribe(ctx)
		if err == nil {
			t.Error("expected error on second subscribe")
		}
	})

	t.Run("closes channel on context cancel", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())

		ch, _ := sub.Subscribe(ctx)
		cancel()

		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected channel to be closed")
			}
		case <-time.After(time.Second):
			t.Error("channel should be closed")
		}
	})
}

func TestSubscriber_ServeHTTP(t *testing.T) {
	t.Run("returns 503 if not subscribed", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})

		req := httptest.NewRequest(http.MethodPost, "/events", nil)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("rejects non-POST", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sub.Subscribe(ctx)

		req := httptest.NewRequest(http.MethodGet, "/events", nil)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("parses single event and delivers to channel", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := sub.Subscribe(ctx)

		// Consume message and ack in background
		go func() {
			msg := <-ch
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := sub.Subscribe(ctx)

		// Nack the message
		go func() {
			msg := <-ch
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sub.Subscribe(ctx)

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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := sub.Subscribe(ctx)

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
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsBatchJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("parses binary mode event", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := sub.Subscribe(ctx)

		var received []byte
		go func() {
			msg := <-ch
			received = msg.Data
			msg.Ack()
		}()

		// Binary mode: data in body, metadata in Ce-* headers
		body := []byte(`{"order_id":"123"}`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Ce-Specversion", "1.0")
		req.Header.Set("Ce-Id", "binary-1")
		req.Header.Set("Ce-Type", "order.created")
		req.Header.Set("Ce-Source", "/orders")
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}
		if string(received) != `{"order_id":"123"}` {
			t.Errorf("expected data to be preserved, got %s", received)
		}
	})
}

func TestSubscriber_ContextCancel(t *testing.T) {
	t.Run("cancel before request returns 503", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		sub.Subscribe(ctx)
		cancel()

		// Wait for shutdown to complete
		time.Sleep(10 * time.Millisecond)

		body := []byte(`{"specversion":"1.0","id":"1","type":"test","source":"/test","data":{}}`)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
		w := httptest.NewRecorder()

		sub.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("cancel during send returns 503", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{BufferSize: 1})
		ctx, cancel := context.WithCancel(context.Background())
		ch, _ := sub.Subscribe(ctx)

		// Fill the buffer so next send blocks
		go func() {
			body := []byte(`{"specversion":"1.0","id":"filler","type":"test","source":"/test","data":{}}`)
			req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
			w := httptest.NewRecorder()
			sub.ServeHTTP(w, req) // This will block waiting for ack
		}()

		// Wait for message to arrive
		<-ch

		// Now send another request that will block on send
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

		// Cancel context - this should unblock the request
		cancel()

		select {
		case code := <-done:
			if code != http.StatusServiceUnavailable {
				t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete after cancel")
		}
	})

	t.Run("cancel during ack wait returns 503", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: 10 * time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		ch, _ := sub.Subscribe(ctx)

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
		msg := <-ch
		_ = msg // Don't ack

		// Cancel context
		cancel()

		select {
		case code := <-done:
			if code != http.StatusServiceUnavailable {
				t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete after cancel")
		}
	})

	t.Run("cancel waits for in-flight requests", func(t *testing.T) {
		sub := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		ch, _ := sub.Subscribe(ctx)

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
			msg := <-ch
			time.Sleep(50 * time.Millisecond) // Simulate processing
			msg.Ack()
		}()

		// Cancel should wait for the request to complete
		cancelDone := make(chan struct{})
		go func() {
			cancel()
			// Channel close happens after wg.Wait(), so wait for channel
			for range ch {
			}
			close(cancelDone)
		}()

		// Request should complete before cancel finishes
		select {
		case <-requestDone:
			// Good - request completed
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete")
		}

		select {
		case <-cancelDone:
			// Good - cancel completed after request finished
		case <-time.After(2 * time.Second):
			t.Fatal("cancel did not complete")
		}
	})
}

func TestSubscriber_WithServeMux(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orders := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})
	payments := NewSubscriber(SubscriberConfig{AckTimeout: time.Second})

	ordersCh, _ := orders.Subscribe(ctx)
	paymentsCh, _ := payments.Subscribe(ctx)

	mux := http.NewServeMux()
	mux.Handle("/events/orders", orders)
	mux.Handle("/events/payments", payments)

	// Consume from both
	go func() {
		msg := <-ordersCh
		msg.Ack()
	}()
	go func() {
		msg := <-paymentsCh
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
