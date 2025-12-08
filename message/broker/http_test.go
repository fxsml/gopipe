package broker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})

	ctx := context.Background()
	payload, _ := json.Marshal("hello world")
	msg := message.New(payload, message.Properties{"gopipe.message.id": "msg-1"})

	if err := sender.Send(ctx, "test/topic", []*message.Message{msg}); err != nil {
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

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})

	ctx := context.Background()
	payload, _ := json.Marshal("test")
	msg := message.New(payload, message.Properties{
		"gopipe.message.id": "id-123",
		"custom.key":        "custom-value",
	})

	if err := sender.Send(ctx, "topic", []*message.Message{msg}); err != nil {
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

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "custom-value",
		},
	})

	ctx := context.Background()
	payload, _ := json.Marshal("test")
	if err := sender.Send(ctx, "topic", []*message.Message{message.New(payload, message.Properties{})}); err != nil {
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

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})

	ctx := context.Background()
	payload, _ := json.Marshal("test")
	err := sender.Send(ctx, "topic", []*message.Message{message.New(payload, message.Properties{})})

	if err == nil {
		t.Fatal("Expected error for 500 response")
	}
}

func TestHTTPReceiver_ServeHTTP(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

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
	msgs, err := receiver.Receive(ctx, "test/topic")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}
	var str string
	json.Unmarshal(msgs[0].Payload, &str)
	if str != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", str)
	}
}

func TestHTTPReceiver_TopicFromPath(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Request with topic in path (no header)
	req := httptest.NewRequest(http.MethodPost, "/orders/created", strings.NewReader(`"order-123"`))
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d", rec.Code)
	}

	msgs, err := receiver.Receive(ctx, "orders/created")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}
	var str string
	json.Unmarshal(msgs[0].Payload, &str)
	if str != "order-123" {
		t.Errorf("Expected 'order-123', got '%s'", str)
	}
}

func TestHTTPReceiver_PropertiesFromHeaders(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`"data"`))
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)
	req.Header.Set(broker.HeaderPrefix+"Prop-Custom-Key", "custom-value")

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	msgs, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}
	// Check property was received
	if v, ok := msgs[0].Properties["custom.key"]; !ok || v != "custom-value" {
		t.Errorf("Expected custom property, got %v", v)
	}
}

func TestHTTPReceiver_MethodNotAllowed(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", rec.Code)
	}
}

func TestHTTPReceiver_MissingTopic(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`"test"`))
	req.Header.Set(broker.HeaderContentType, broker.ContentTypeJSON)

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rec.Code)
	}
}

func TestHTTPReceiver_AnyPayload(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx := context.Background()

	// HTTP receiver accepts any payload (raw bytes)
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`not valid json`))
	req.Header.Set(broker.HeaderTopic, "test")

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	// Should accept any payload
	if rec.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d", rec.Code)
	}

	// Verify payload is stored as-is
	msgs, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}
	if string(msgs[0].Payload) != "not valid json" {
		t.Errorf("Expected raw payload, got %s", msgs[0].Payload)
	}
}

func TestHTTPReceiver_Close(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)

	ctx := context.Background()

	if err := receiver.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Receive should fail after close
	_, err := receiver.Receive(ctx, "test")
	if err != broker.ErrHTTPServerClosed {
		t.Errorf("Expected ErrHTTPServerClosed, got %v", err)
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
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send to specific topics
	req := httptest.NewRequest(http.MethodPost, "/orders/created", strings.NewReader(`"order1"`))
	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)

	req2 := httptest.NewRequest(http.MethodPost, "/orders/updated", strings.NewReader(`"order2"`))
	rec2 := httptest.NewRecorder()
	receiver.ServeHTTP(rec2, req2)

	// Receive with wildcard - should get both
	msgs, err := receiver.Receive(ctx, "orders/+")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}
}

func TestHTTP_RoundTrip(t *testing.T) {
	// Create receiver with HTTP server
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	server := httptest.NewServer(receiver.Handler())
	defer server.Close()

	// Create sender pointing to server
	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send
	payload, _ := json.Marshal("round-trip-test")
	msg := message.New(payload, message.Properties{"key": "value"})
	if err := sender.Send(ctx, "test", []*message.Message{msg}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive
	msgs, err := receiver.Receive(ctx, "test")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}
	var str string
	json.Unmarshal(msgs[0].Payload, &str)
	if str != "round-trip-test" {
		t.Errorf("Expected 'round-trip-test', got '%s'", str)
	}
}

func TestHTTP_StructPayload(t *testing.T) {
	type Event struct {
		Name string `json:"name"`
		Data int    `json:"data"`
	}

	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 100)
	defer receiver.Close()

	server := httptest.NewServer(receiver.Handler())
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	event := Event{Name: "test-event", Data: 42}
	payload, _ := json.Marshal(event)
	if err := sender.Send(ctx, "events", []*message.Message{message.New(payload, message.Properties{})}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	msgs, err := receiver.Receive(ctx, "events")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("Expected message")
	}

	var receivedEvent Event
	json.Unmarshal(msgs[0].Payload, &receivedEvent)
	if receivedEvent.Name != "test-event" || receivedEvent.Data != 42 {
		t.Errorf("Payload mismatch: %+v", receivedEvent)
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
