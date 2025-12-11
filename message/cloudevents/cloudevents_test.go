package cloudevents_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/cloudevents"
)

func TestFromMessage_Basic(t *testing.T) {
	msg := message.New([]byte(`{"test":"data"}`), message.Attributes{
		message.AttrID:     "test-001",
		message.AttrSource: "test-source",
		message.AttrType:   "test.event",
	})

	event := cloudevents.FromMessage(msg, "test.topic", "default-source")

	if event.ID != "test-001" {
		t.Errorf("Expected ID test-001, got %s", event.ID)
	}
	if event.Source != "test-source" {
		t.Errorf("Expected source test-source, got %s", event.Source)
	}
	if event.Type != "test.event" {
		t.Errorf("Expected type test.event, got %s", event.Type)
	}
	if event.SpecVersion != cloudevents.SpecVersion {
		t.Errorf("Expected specversion %s, got %s", cloudevents.SpecVersion, event.SpecVersion)
	}
	if topic, ok := event.Extensions["topic"].(string); !ok || topic != "test.topic" {
		t.Errorf("Expected topic test.topic in extensions, got %v", event.Extensions["topic"])
	}
}

func TestFromMessage_Defaults(t *testing.T) {
	msg := message.New([]byte(`{"test":"data"}`), message.Attributes{})

	event := cloudevents.FromMessage(msg, "test.topic", "default-source")

	// Should generate ID if not provided
	if event.ID == "" {
		t.Error("Expected generated ID, got empty string")
	}
	// Should use default source
	if event.Source != "default-source" {
		t.Errorf("Expected default-source, got %s", event.Source)
	}
	// Should use default type
	if event.Type != "com.gopipe.message" {
		t.Errorf("Expected default type com.gopipe.message, got %s", event.Type)
	}
}

func TestFromMessage_JSONData(t *testing.T) {
	msg := message.New([]byte(`{"order":"123","total":45.99}`), message.Attributes{
		message.AttrDataContentType: "application/json",
	})

	event := cloudevents.FromMessage(msg, "orders.created", "test-source")

	// Data should be unmarshaled as JSON object
	dataMap, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map[string]interface{}, got %T", event.Data)
	}
	if dataMap["order"] != "123" {
		t.Errorf("Expected order 123, got %v", dataMap["order"])
	}
}

func TestFromMessage_BinaryData(t *testing.T) {
	// Binary data (contains control characters)
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	msg := message.New(binaryData, message.Attributes{
		message.AttrDataContentType: "application/octet-stream",
	})

	event := cloudevents.FromMessage(msg, "test.topic", "test-source")

	// Should use data_base64 for binary data
	if event.DataBase64 == "" {
		t.Error("Expected data_base64 to be set for binary data")
	}
	if event.Data != nil && event.Data != "" {
		t.Error("Expected data to be empty when data_base64 is set")
	}
}

func TestFromMessage_Extensions(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	msg := message.New([]byte("test"), message.Attributes{
		message.AttrCorrelationID: "corr-123",
		message.AttrDeadline:      deadline,
		"custom":                  "value",
	})

	event := cloudevents.FromMessage(msg, "test.topic", "test-source")

	if event.Extensions["correlationid"] != "corr-123" {
		t.Errorf("Expected correlationid corr-123, got %v", event.Extensions["correlationid"])
	}
	if event.Extensions["topic"] != "test.topic" {
		t.Errorf("Expected topic test.topic, got %v", event.Extensions["topic"])
	}
	if event.Extensions["custom"] != "value" {
		t.Errorf("Expected custom value, got %v", event.Extensions["custom"])
	}
	// Deadline should be formatted as RFC3339Nano string
	deadlineStr, ok := event.Extensions["deadline"].(string)
	if !ok {
		t.Errorf("Expected deadline to be string, got %T", event.Extensions["deadline"])
	} else {
		parsed, err := time.Parse(time.RFC3339Nano, deadlineStr)
		if err != nil {
			t.Errorf("Failed to parse deadline: %v", err)
		}
		if !parsed.Equal(deadline) {
			t.Errorf("Deadline mismatch: expected %v, got %v", deadline, parsed)
		}
	}
}

func TestToMessage_Basic(t *testing.T) {
	event := &cloudevents.Event{
		ID:          "test-001",
		Source:      "test-source",
		SpecVersion: "1.0",
		Type:        "test.event",
		Data:        map[string]string{"test": "data"},
		Extensions: map[string]any{
			"topic": "test.topic",
		},
	}

	msg, topic, err := cloudevents.ToMessage(event)
	if err != nil {
		t.Fatalf("ToMessage failed: %v", err)
	}

	if topic != "test.topic" {
		t.Errorf("Expected topic test.topic, got %s", topic)
	}

	id, _ := msg.Attributes.ID()
	if id != "test-001" {
		t.Errorf("Expected ID test-001, got %s", id)
	}

	// Data should be marshaled back to JSON
	var dataMap map[string]string
	if err := json.Unmarshal(msg.Data, &dataMap); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}
	if dataMap["test"] != "data" {
		t.Errorf("Expected test=data, got %v", dataMap)
	}
}

func TestToMessage_RequiredAttributes(t *testing.T) {
	tests := []struct {
		name  string
		event *cloudevents.Event
	}{
		{
			name: "missing ID",
			event: &cloudevents.Event{
				Source:      "test",
				SpecVersion: "1.0",
				Type:        "test",
			},
		},
		{
			name: "missing Source",
			event: &cloudevents.Event{
				ID:          "test",
				SpecVersion: "1.0",
				Type:        "test",
			},
		},
		{
			name: "missing Type",
			event: &cloudevents.Event{
				ID:          "test",
				Source:      "test",
				SpecVersion: "1.0",
			},
		},
		{
			name: "missing SpecVersion",
			event: &cloudevents.Event{
				ID:     "test",
				Source: "test",
				Type:   "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := cloudevents.ToMessage(tt.event)
			if err == nil {
				t.Error("Expected error for missing required attribute")
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := message.New([]byte(`"test string"`), message.Attributes{
		message.AttrID:              "test-001",
		message.AttrSource:          "test-source",
		message.AttrType:            "test.event",
		message.AttrSubject:         "test-subject",
		message.AttrDataContentType: "application/json",
		message.AttrCorrelationID:   "corr-123",
	})

	// Convert to CloudEvent
	event := cloudevents.FromMessage(original, "test.topic", "default-source")

	// Marshal to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal from JSON
	var parsed cloudevents.Event
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Convert back to message
	result, topic, err := cloudevents.ToMessage(&parsed)
	if err != nil {
		t.Fatalf("ToMessage failed: %v", err)
	}

	// Verify topic
	if topic != "test.topic" {
		t.Errorf("Expected topic test.topic, got %s", topic)
	}

	// Verify data (should preserve JSON encoding)
	if string(result.Data) != string(original.Data) {
		t.Errorf("Data mismatch: expected %s, got %s", original.Data, result.Data)
	}

	// Verify attributes
	id, _ := result.Attributes.ID()
	if id != "test-001" {
		t.Errorf("Expected ID test-001, got %s", id)
	}

	source, _ := result.Attributes.Source()
	if source != "test-source" {
		t.Errorf("Expected source test-source, got %s", source)
	}

	correlationID, _ := result.Attributes.CorrelationID()
	if correlationID != "corr-123" {
		t.Errorf("Expected correlationID corr-123, got %s", correlationID)
	}
}

func TestMarshalJSON_Extensions(t *testing.T) {
	event := &cloudevents.Event{
		ID:          "test-001",
		Source:      "test-source",
		SpecVersion: "1.0",
		Type:        "test.event",
		Extensions: map[string]any{
			"topic":  "test.topic",
			"custom": "value",
		},
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify extensions are at top level
	var m map[string]any
	if err := json.Unmarshal(jsonData, &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if m["topic"] != "test.topic" {
		t.Errorf("Expected topic at top level, got %v", m["topic"])
	}
	if m["custom"] != "value" {
		t.Errorf("Expected custom at top level, got %v", m["custom"])
	}
}

func TestUnmarshalJSON_Extensions(t *testing.T) {
	jsonStr := `{
		"id": "test-001",
		"source": "test-source",
		"specversion": "1.0",
		"type": "test.event",
		"topic": "test.topic",
		"custom": "value"
	}`

	var event cloudevents.Event
	if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if event.Extensions["topic"] != "test.topic" {
		t.Errorf("Expected topic in extensions, got %v", event.Extensions["topic"])
	}
	if event.Extensions["custom"] != "value" {
		t.Errorf("Expected custom in extensions, got %v", event.Extensions["custom"])
	}
}
