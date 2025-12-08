package broker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func TestHTTPSender_Send(t *testing.T) {
	var receivedReq *http.Request
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedReq = r
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	sender := broker.NewHTTPSender[string](server.URL, broker.HTTPConfig{})

	ctx := context.Background()
	msg := message.New("hello world", message.WithID[string]("msg-1"))

	if err := sender.Send(ctx, "test/topic", msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify request
	if receivedReq.Method != http.MethodPost {
		t.Errorf("Expected POST, got %s", receivedReq.Method)
	}
	if receivedReq.Header.Get(broker.HeaderTopic) != "test/topic" {
		t.Errorf("Expected topic header, got %s", receivedReq.Header.Get(broker.HeaderTopic))
	}
	if receivedReq.Header.Get(broker.HeaderContentType) != broker.ContentTypeJSON {
		t.Errorf("Expected JSON content type, got %s", receivedReq.Header.Get(broker.HeaderContentType))
	}
	if !strings.Contains(receivedBody, "hello world") {
		t.Errorf("Expected payload in body, got %s", receivedBody)
	}
}

func TestHTTPSender_Properties(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	sender := broker.NewHTTPSender[string](server.URL, broker.HTTPConfig{})

	ctx := context.Background()
	msg := message.New("test",
		message.WithID[string]("id-123"),
		message.WithProperty[string]("custom.key", "custom-value"),
	)

	if err := sender.Send(ctx, "topic", msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Check that properties are in headers
	found := false
	for key := range receivedHeaders {
		if strings.HasPrefix(key, broker.HeaderPrefix+"Prop-") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected property headers")
	}
}

func TestHTTPSender_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	sender := broker.NewHTTPSender[string](server.URL, broker.HTTPConfig{
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "custom-value",
		},
	})

	ctx := context.Background()
	if err := sender.Send(ctx, "topic", message.New("test")); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Expected Authorization header")
	}
	if receivedHeaders.Get("X-Custom") != "custom-value" {
		t.Errorf("Expected X-Custom header")
	}
}

func TestHTTPSender_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender := broker.NewHTTPSender[string](server.URL, broker.HTTPConfig{})

	ctx := context.Background()
	err := sender.Send(ctx, "topic", message.New("test"))

	if err == nil {
		t.Fatal("Expected error for 500 response")
	}
}

func TestHTTPReceiver_ServeHTTP(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Subscribe to topic
	msgs := receiver.Receive(ctx, "test/topic")

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`"hello world"`))
	req.Header.Set(broker.HeaderTopic, "test/topic")
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	// Check response
	if rec.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d", rec.Code)
	}

	// Check message received
	select {
	case msg := <-msgs:
		if msg.Payload() != "hello world" {
			t.Errorf("Expected 'hello world', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

func TestHTTPReceiver_TopicFromPath(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := receiver.Receive(ctx, "orders/created")

	// Request with topic in path (no header)
	req := httptest.NewRequest(http.MethodPost, "/orders/created", strings.NewReader(`"order-123"`))
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d", rec.Code)
	}

	select {
	case msg := <-msgs:
		if msg.Payload() != "order-123" {
			t.Errorf("Expected 'order-123', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}
}

func TestHTTPReceiver_PropertiesFromHeaders(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := receiver.Receive(ctx, "test")

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`"data"`))
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)
	req.Header.Set(broker.HeaderPrefix+"Prop-Custom-Key", "custom-value")

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	select {
	case msg := <-msgs:
		// Check property was received
		if v, ok := msg.Properties().Get("custom.key"); !ok || v != "custom-value" {
			t.Errorf("Expected custom property, got %v", v)
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}
}

func TestHTTPReceiver_MethodNotAllowed(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", rec.Code)
	}
}

func TestHTTPReceiver_MissingTopic(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`"test"`))
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rec.Code)
	}
}

func TestHTTPReceiver_InvalidPayload(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`not valid json`))
	req.Header.Set(broker.HeaderTopic, "test")

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rec.Code)
	}
}

func TestHTTPReceiver_Close(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)

	ctx := context.Background()
	msgs := receiver.Receive(ctx, "test")

	if err := receiver.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Channel should be closed
	select {
	case _, ok := <-msgs:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for channel close")
	}

	// Double close should error
	if err := receiver.Close(); err != broker.ErrHTTPServerClosed {
		t.Errorf("Expected ErrHTTPServerClosed, got %v", err)
	}

	// Requests should fail after close
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`"test"`))
	req.Header.Set(broker.HeaderTopic, "test")
	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", rec.Code)
	}
}

func TestHTTPReceiver_WildcardTopic(t *testing.T) {
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Subscribe with wildcard
	msgs := receiver.Receive(ctx, "orders/+")

	// Send to specific topic
	req := httptest.NewRequest(http.MethodPost, "/orders/created", strings.NewReader(`"order1"`))
	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	req2 := httptest.NewRequest(http.MethodPost, "/orders/updated", strings.NewReader(`"order2"`))
	rec2 := httptest.NewRecorder()
	receiver.ServeHTTP(rec2, req2)

	// Should receive both
	received := 0
	timeout := time.After(500 * time.Millisecond)
	for received < 2 {
		select {
		case <-msgs:
			received++
		case <-timeout:
			t.Fatalf("Timeout after %d messages", received)
		}
	}
}

func TestHTTP_RoundTrip(t *testing.T) {
	// Create receiver with HTTP server
	receiver := broker.NewHTTPReceiver[string](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	server := httptest.NewServer(receiver.Handler())
	defer server.Close()

	// Create sender pointing to server
	sender := broker.NewHTTPSender[string](server.URL, broker.HTTPConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Subscribe
	msgs := receiver.Receive(ctx, "test")

	// Send
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond) // Wait for subscription
		sender.Send(ctx, "test", message.New("round-trip-test",
			message.WithProperty[string]("key", "value"),
		))
	}()

	// Receive
	select {
	case msg := <-msgs:
		if msg.Payload() != "round-trip-test" {
			t.Errorf("Expected 'round-trip-test', got '%s'", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	wg.Wait()
}

func TestHTTP_StructPayload(t *testing.T) {
	type Event struct {
		Name string `json:"name"`
		Data int    `json:"data"`
	}

	receiver := broker.NewHTTPReceiver[Event](broker.HTTPConfig{}, 100)
	defer receiver.Close()

	server := httptest.NewServer(receiver.Handler())
	defer server.Close()

	sender := broker.NewHTTPSender[Event](server.URL, broker.HTTPConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs := receiver.Receive(ctx, "events")
	time.Sleep(20 * time.Millisecond)

	event := Event{Name: "test-event", Data: 42}
	sender.Send(ctx, "events", message.New(event))

	select {
	case msg := <-msgs:
		if msg.Payload().Name != "test-event" || msg.Payload().Data != 42 {
			t.Errorf("Payload mismatch: %+v", msg.Payload())
		}
	case <-ctx.Done():
		t.Fatal("Timeout")
	}
}

func TestTopicFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/orders/created", "orders/created"},
		{"/", ""},
		{"/a/b/c", "a/b/c"},
		{"/url%2Fencoded", "url/encoded"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := broker.TopicFromPath(tt.path)
			if got != tt.want {
				t.Errorf("TopicFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
