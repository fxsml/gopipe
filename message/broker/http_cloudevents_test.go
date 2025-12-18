package broker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/broker"
)

func TestHTTPCloudEvents_BinaryMode(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		CloudEventsMode: broker.CloudEventsBinary,
		ContentType:     "application/json",
	})

	ctx := context.Background()

	// Create a message with CloudEvents attributes
	msg := message.New([]byte(`{"order":"123"}`), message.Attributes{
		message.AttrID:              "evt-001",
		message.AttrSource:          "https://example.com/orders",
		message.AttrType:            "com.example.order.created",
		message.AttrSubject:         "order/123",
		message.AttrTime:            time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		message.AttrDataContentType: "application/json",
	})

	err := sender.Send(ctx, "orders.created", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive the message
	received, err := receiver.Receive(ctx, "orders.created")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(received))
	}

	// Verify attributes
	rcvMsg := received[0]
	if id, _ := rcvMsg.Attributes.ID(); id != "evt-001" {
		t.Errorf("Expected id 'evt-001', got '%s'", id)
	}

	if source, _ := rcvMsg.Attributes.Source(); source != "https://example.com/orders" {
		t.Errorf("Expected source 'https://example.com/orders', got '%s'", source)
	}

	if eventType, _ := rcvMsg.Attributes.Type(); eventType != "com.example.order.created" {
		t.Errorf("Expected type 'com.example.order.created', got '%s'", eventType)
	}

	if subject, _ := rcvMsg.Attributes.Subject(); subject != "order/123" {
		t.Errorf("Expected subject 'order/123', got '%s'", subject)
	}

	// Verify data
	if string(rcvMsg.Data) != `{"order":"123"}` {
		t.Errorf("Expected data '{\"order\":\"123\"}', got '%s'", string(rcvMsg.Data))
	}
}

func TestHTTPCloudEvents_BinaryMode_PercentEncoding(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		CloudEventsMode: broker.CloudEventsBinary,
	})

	ctx := context.Background()

	// Create message with special characters requiring percent-encoding
	msg := message.New([]byte(`{}`), message.Attributes{
		message.AttrID:      "evt 001",               // Contains space
		message.AttrSource:  "https://example.com/€", // Contains special char
		message.AttrType:    "test.type",
		message.AttrSubject: "Euro € 😀", // Contains non-ASCII characters
	})

	err := sender.Send(ctx, "test.topic", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive and verify
	received, err := receiver.Receive(ctx, "test.topic")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(received))
	}

	rcvMsg := received[0]
	if id, _ := rcvMsg.Attributes.ID(); id != "evt 001" {
		t.Errorf("Expected id 'evt 001', got '%s'", id)
	}

	if subject, _ := rcvMsg.Attributes.Subject(); subject != "Euro € 😀" {
		t.Errorf("Expected subject 'Euro € 😀', got '%s'", subject)
	}
}

func TestHTTPCloudEvents_StructuredMode(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		CloudEventsMode: broker.CloudEventsStructured,
	})

	ctx := context.Background()

	// Create a message with CloudEvents attributes
	msg := message.New([]byte(`{"order":"456"}`), message.Attributes{
		message.AttrID:              "evt-002",
		message.AttrSource:          "https://example.com/payments",
		message.AttrType:            "com.example.payment.completed",
		message.AttrSubject:         "payment/456",
		message.AttrDataContentType: "application/json",
	})

	err := sender.Send(ctx, "payments.completed", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive the message
	received, err := receiver.Receive(ctx, "payments.completed")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(received))
	}

	// Verify attributes
	rcvMsg := received[0]
	if id, _ := rcvMsg.Attributes.ID(); id != "evt-002" {
		t.Errorf("Expected id 'evt-002', got '%s'", id)
	}

	if source, _ := rcvMsg.Attributes.Source(); source != "https://example.com/payments" {
		t.Errorf("Expected source 'https://example.com/payments', got '%s'", source)
	}

	// Verify data - should be unmarshaled as JSON
	var data map[string]interface{}
	if err := json.Unmarshal(rcvMsg.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if data["order"] != "456" {
		t.Errorf("Expected order '456', got '%v'", data["order"])
	}
}

func TestHTTPCloudEvents_BatchMode(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		CloudEventsMode: broker.CloudEventsBatch,
	})

	ctx := context.Background()

	// Create multiple messages
	messages := []*message.Message{
		message.New([]byte(`{"order":"1"}`), message.Attributes{
			message.AttrID:     "evt-1",
			message.AttrSource: "https://example.com/orders",
			message.AttrType:   "com.example.order.created",
		}),
		message.New([]byte(`{"order":"2"}`), message.Attributes{
			message.AttrID:     "evt-2",
			message.AttrSource: "https://example.com/orders",
			message.AttrType:   "com.example.order.created",
		}),
		message.New([]byte(`{"order":"3"}`), message.Attributes{
			message.AttrID:     "evt-3",
			message.AttrSource: "https://example.com/orders",
			message.AttrType:   "com.example.order.created",
		}),
	}

	err := sender.Send(ctx, "orders.batch", messages)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive the messages
	received, err := receiver.Receive(ctx, "orders.batch")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(received))
	}

	// Verify each message
	for i, msg := range received {
		expectedID := []string{"evt-1", "evt-2", "evt-3"}[i]
		if id, _ := msg.Attributes.ID(); id != expectedID {
			t.Errorf("Message %d: expected id '%s', got '%s'", i, expectedID, id)
		}
	}
}

func TestHTTPCloudEvents_ExtensionAttributes(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		CloudEventsMode: broker.CloudEventsBinary,
	})

	ctx := context.Background()

	// Create message with gopipe extension attributes
	msg := message.New([]byte(`{}`), message.Attributes{
		message.AttrID:            "evt-ext",
		message.AttrSource:        "test-source",
		message.AttrType:          "test.type",
		message.AttrCorrelationID: "corr-123",
		message.AttrDeadline:      time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
		"customext":               "custom-value",
	})

	err := sender.Send(ctx, "test.extensions", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	received, err := receiver.Receive(ctx, "test.extensions")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(received))
	}

	rcvMsg := received[0]

	// Verify correlation ID
	if correlationID, ok := rcvMsg.Attributes.CorrelationID(); !ok || correlationID != "corr-123" {
		t.Errorf("Expected correlationid 'corr-123', got '%s'", correlationID)
	}

	// Verify deadline
	if deadline, ok := rcvMsg.Attributes.Deadline(); !ok {
		t.Error("Expected deadline to be set")
	} else {
		expected := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
		if !deadline.Equal(expected) {
			t.Errorf("Expected deadline %v, got %v", expected, deadline)
		}
	}

	// Verify custom extension
	if customExt, ok := rcvMsg.Attributes["customext"].(string); !ok || customExt != "custom-value" {
		t.Errorf("Expected customext 'custom-value', got '%v'", rcvMsg.Attributes["customext"])
	}
}

func TestHTTPCloudEvents_DefaultAttributes(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	sender := broker.NewHTTPSender(server.URL, broker.HTTPConfig{
		CloudEventsMode: broker.CloudEventsBinary,
	})

	ctx := context.Background()

	// Create message without required attributes - should be auto-generated
	msg := message.New([]byte(`{"test":"data"}`), message.Attributes{})

	err := sender.Send(ctx, "test.defaults", []*message.Message{msg})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	received, err := receiver.Receive(ctx, "test.defaults")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(received))
	}

	rcvMsg := received[0]

	// Verify auto-generated ID
	if id, ok := rcvMsg.Attributes.ID(); !ok || id == "" {
		t.Error("Expected auto-generated ID")
	}

	// Verify default source (should be the sender URL)
	if source, ok := rcvMsg.Attributes.Source(); !ok || source == "" {
		t.Error("Expected default source")
	}

	// Verify default type
	if eventType, ok := rcvMsg.Attributes.Type(); !ok || eventType != "com.gopipe.message" {
		t.Errorf("Expected default type 'com.pipe.message', got '%s'", eventType)
	}

	// Verify specversion
	if specversion, ok := rcvMsg.Attributes.SpecVersion(); !ok || specversion != "1.0" {
		t.Errorf("Expected specversion '1.0', got '%s'", specversion)
	}
}

func TestHTTPCloudEvents_ReceiverDetectsMode(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	ctx := context.Background()

	t.Run("Binary mode detected from headers", func(t *testing.T) {
		ceEvent := map[string]string{
			"Ce-Id":          "test-binary",
			"Ce-Source":      "test-source",
			"Ce-Type":        "test.type",
			"Ce-Specversion": "1.0",
			"Ce-Topic":       "binary.topic",
			"Content-Type":   "application/json",
		}

		req, _ := http.NewRequest("POST", server.URL, bytes.NewReader([]byte(`{"test":"binary"}`)))
		for k, v := range ceEvent {
			req.Header.Set(k, v)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		// Verify message was received
		received, _ := receiver.Receive(ctx, "binary.topic")
		if len(received) != 1 {
			t.Errorf("Expected 1 message, got %d", len(received))
		}
	})

	t.Run("Structured mode detected from Content-Type", func(t *testing.T) {
		ceJSON := map[string]interface{}{
			"id":          "test-structured",
			"source":      "test-source",
			"type":        "test.type",
			"specversion": "1.0",
			"topic":       "structured.topic",
			"data":        map[string]string{"test": "structured"},
		}

		body, _ := json.Marshal(ceJSON)
		req, _ := http.NewRequest("POST", server.URL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/cloudevents+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		// Verify message was received
		received, _ := receiver.Receive(ctx, "structured.topic")
		if len(received) != 1 {
			t.Errorf("Expected 1 message, got %d", len(received))
		}
	})

	t.Run("Batch mode detected from Content-Type", func(t *testing.T) {
		batch := []map[string]interface{}{
			{
				"id":          "test-batch-1",
				"source":      "test-source",
				"type":        "test.type",
				"specversion": "1.0",
				"topic":       "batch.topic",
				"data":        "batch1",
			},
			{
				"id":          "test-batch-2",
				"source":      "test-source",
				"type":        "test.type",
				"specversion": "1.0",
				"topic":       "batch.topic",
				"data":        "batch2",
			},
		}

		body, _ := json.Marshal(batch)
		req, _ := http.NewRequest("POST", server.URL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/cloudevents-batch+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		// Verify messages were received
		received, _ := receiver.Receive(ctx, "batch.topic")
		if len(received) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(received))
		}
	})
}

func TestHTTPCloudEvents_InvalidEvent(t *testing.T) {
	receiver := broker.NewHTTPReceiver(broker.HTTPConfig{}, 10)
	defer receiver.Close()

	server := httptest.NewServer(receiver)
	defer server.Close()

	t.Run("Missing required attribute in binary mode", func(t *testing.T) {
		req, _ := http.NewRequest("POST", server.URL, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Ce-Id", "test-id")
		req.Header.Set("Ce-Source", "test-source")
		// Missing Ce-Type (required)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid JSON in structured mode", func(t *testing.T) {
		req, _ := http.NewRequest("POST", server.URL, bytes.NewReader([]byte(`{invalid json`)))
		req.Header.Set("Content-Type", "application/cloudevents+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}
