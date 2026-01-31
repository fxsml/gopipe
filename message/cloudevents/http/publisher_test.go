package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
)

func TestPublisher_Send(t *testing.T) {
	t.Run("sends single event", func(t *testing.T) {
		var received []byte
		var contentType string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			received, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{
			TargetURL: server.URL,
		})

		msg := message.NewRaw([]byte(`{"key":"value"}`), message.Attributes{
			message.AttrID:     "test-1",
			message.AttrType:   "test.type",
			message.AttrSource: "/test",
		}, nil)

		err := pub.Send(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contentType != ContentTypeCloudEventsJSON {
			t.Errorf("expected content-type %s, got %s", ContentTypeCloudEventsJSON, contentType)
		}

		if len(received) == 0 {
			t.Error("expected non-empty body")
		}
	})

	t.Run("acks on 2xx response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		var acked bool
		acking := message.NewAcking(func() { acked = true }, func(error) {})
		msg := message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     "1",
			message.AttrType:   "test",
			message.AttrSource: "/test",
		}, acking)

		err := pub.Send(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !acked {
			t.Error("expected message to be acked")
		}
	})

	t.Run("nacks on error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		var nacked bool
		acking := message.NewAcking(func() {}, func(error) { nacked = true })
		msg := message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     "1",
			message.AttrType:   "test",
			message.AttrSource: "/test",
		}, acking)

		err := pub.Send(context.Background(), msg)
		if err == nil {
			t.Fatal("expected error")
		}

		if !nacked {
			t.Error("expected message to be nacked")
		}
	})

	t.Run("includes custom headers", func(t *testing.T) {
		var authHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{
			TargetURL: server.URL,
			Headers:   http.Header{"Authorization": []string{"Bearer token"}},
		})

		msg := message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     "1",
			message.AttrType:   "test",
			message.AttrSource: "/test",
		}, nil)

		_ = pub.Send(context.Background(), msg)

		if authHeader != "Bearer token" {
			t.Errorf("expected 'Bearer token', got %s", authHeader)
		}
	})
}

func TestPublisher_SendBatch(t *testing.T) {
	t.Run("sends batch with correct content type", func(t *testing.T) {
		var contentType string
		var bodySize int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			bodySize = len(body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		msgs := []*message.RawMessage{
			message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID: "1", message.AttrType: "test", message.AttrSource: "/test",
			}, nil),
			message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID: "2", message.AttrType: "test", message.AttrSource: "/test",
			}, nil),
		}

		err := pub.SendBatch(context.Background(), msgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contentType != ContentTypeCloudEventsBatchJSON {
			t.Errorf("expected %s, got %s", ContentTypeCloudEventsBatchJSON, contentType)
		}

		if bodySize == 0 {
			t.Error("expected non-empty body")
		}
	})

	t.Run("acks all messages on success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		var ackCount atomic.Int32
		msgs := make([]*message.RawMessage, 3)
		for i := range msgs {
			acking := message.NewAcking(func() { ackCount.Add(1) }, func(error) {})
			msgs[i] = message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID: "1", message.AttrType: "test", message.AttrSource: "/test",
			}, acking)
		}

		err := pub.SendBatch(context.Background(), msgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ackCount.Load() != 3 {
			t.Errorf("expected 3 acks, got %d", ackCount.Load())
		}
	})

	t.Run("nacks all messages on failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		var nackCount atomic.Int32
		msgs := make([]*message.RawMessage, 3)
		for i := range msgs {
			acking := message.NewAcking(func() {}, func(error) { nackCount.Add(1) })
			msgs[i] = message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID: "1", message.AttrType: "test", message.AttrSource: "/test",
			}, acking)
		}

		err := pub.SendBatch(context.Background(), msgs)
		if err == nil {
			t.Fatal("expected error")
		}

		if nackCount.Load() != 3 {
			t.Errorf("expected 3 nacks, got %d", nackCount.Load())
		}
	})

	t.Run("handles empty batch", func(t *testing.T) {
		pub := NewPublisher(PublisherConfig{TargetURL: "http://localhost"})
		err := pub.SendBatch(context.Background(), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestPublisher_Publish(t *testing.T) {
	t.Run("single mode sends individually", func(t *testing.T) {
		var count atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{
			TargetURL:   server.URL,
			Concurrency: 2,
		})

		ch := make(chan *message.RawMessage, 10)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, err := pub.Publish(ctx, ch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			ch <- message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID:     "1",
				message.AttrType:   "test",
				message.AttrSource: "/test",
			}, nil)
		}
		close(ch)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}

		if count.Load() != 5 {
			t.Errorf("expected 5 requests, got %d", count.Load())
		}
	})

	t.Run("batch mode batches messages", func(t *testing.T) {
		var requestCount atomic.Int32
		var batchSizes []int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			msgs, _ := ParseBatchBytes(body)
			batchSizes = append(batchSizes, len(msgs))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{
			TargetURL:     server.URL,
			BatchSize:     3,
			BatchDuration: 10 * time.Second,
		})

		ch := make(chan *message.RawMessage, 100)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, err := pub.Publish(ctx, ch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Send 7 messages (should result in batches of 3, 3, 1)
		for i := 0; i < 7; i++ {
			ch <- message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID:     "1",
				message.AttrType:   "test",
				message.AttrSource: "/test",
			}, nil)
		}
		close(ch)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}

		// Should have fewer requests than messages (batching)
		if requestCount.Load() >= 7 {
			t.Errorf("expected batching, got %d requests for 7 messages", requestCount.Load())
		}
	})

	t.Run("returns error if called twice", func(t *testing.T) {
		pub := NewPublisher(PublisherConfig{TargetURL: "http://localhost"})
		ch := make(chan *message.RawMessage)
		ctx := context.Background()

		_, err := pub.Publish(ctx, ch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = pub.Publish(ctx, ch)
		if err == nil {
			t.Fatal("expected error on second call")
		}
	})
}
