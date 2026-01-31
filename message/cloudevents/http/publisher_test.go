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

func TestPublisher_PublishOne(t *testing.T) {
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

		err := pub.PublishOne(context.Background(), msg)
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

		err := pub.PublishOne(context.Background(), msg)
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

		err := pub.PublishOne(context.Background(), msg)
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

		_ = pub.PublishOne(context.Background(), msg)

		if authHeader != "Bearer token" {
			t.Errorf("expected 'Bearer token', got %s", authHeader)
		}
	})
}

func TestPublisher_Publish(t *testing.T) {
	t.Run("publishes messages from channel", func(t *testing.T) {
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

		// Send messages
		for i := 0; i < 5; i++ {
			ch <- message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID:     "1",
				message.AttrType:   "test",
				message.AttrSource: "/test",
			}, nil)
		}
		close(ch)

		// Wait for completion
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for publish to complete")
		}

		if count.Load() != 5 {
			t.Errorf("expected 5 requests, got %d", count.Load())
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

func TestPublisher_PublishBatch(t *testing.T) {
	t.Run("batches by size", func(t *testing.T) {
		var batchSizes []int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			msgs, _ := ParseBatchBytes(body)
			batchSizes = append(batchSizes, len(msgs))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		ch := make(chan *message.RawMessage, 100)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, err := pub.PublishBatch(ctx, ch, BatchConfig{
			MaxSize:     3,
			MaxDuration: 10 * time.Second, // Long duration so size triggers
		})
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

		if len(batchSizes) < 2 {
			t.Errorf("expected at least 2 batches, got %d", len(batchSizes))
		}
	})

	t.Run("uses batch content type", func(t *testing.T) {
		var contentType string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		ch := make(chan *message.RawMessage, 10)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, _ := pub.PublishBatch(ctx, ch, BatchConfig{
			MaxSize:     10,
			MaxDuration: 100 * time.Millisecond,
		})

		ch <- message.NewRaw([]byte(`{}`), message.Attributes{
			message.AttrID:     "1",
			message.AttrType:   "test",
			message.AttrSource: "/test",
		}, nil)
		close(ch)

		<-done

		if contentType != ContentTypeCloudEventsBatchJSON {
			t.Errorf("expected %s, got %s", ContentTypeCloudEventsBatchJSON, contentType)
		}
	})

	t.Run("acks all messages in batch on success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		ch := make(chan *message.RawMessage, 10)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, _ := pub.PublishBatch(ctx, ch, BatchConfig{
			MaxSize:     10,
			MaxDuration: 100 * time.Millisecond,
		})

		var ackCount atomic.Int32
		for i := 0; i < 3; i++ {
			acking := message.NewAcking(func() { ackCount.Add(1) }, func(error) {})
			ch <- message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID:     "1",
				message.AttrType:   "test",
				message.AttrSource: "/test",
			}, acking)
		}
		close(ch)

		<-done

		if ackCount.Load() != 3 {
			t.Errorf("expected 3 acks, got %d", ackCount.Load())
		}
	})

	t.Run("nacks all messages in batch on failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		pub := NewPublisher(PublisherConfig{TargetURL: server.URL})

		ch := make(chan *message.RawMessage, 10)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done, _ := pub.PublishBatch(ctx, ch, BatchConfig{
			MaxSize:     10,
			MaxDuration: 100 * time.Millisecond,
		})

		var nackCount atomic.Int32
		for i := 0; i < 3; i++ {
			acking := message.NewAcking(func() {}, func(error) { nackCount.Add(1) })
			ch <- message.NewRaw([]byte(`{}`), message.Attributes{
				message.AttrID:     "1",
				message.AttrType:   "test",
				message.AttrSource: "/test",
			}, acking)
		}
		close(ch)

		<-done

		if nackCount.Load() != 3 {
			t.Errorf("expected 3 nacks, got %d", nackCount.Load())
		}
	})
}
