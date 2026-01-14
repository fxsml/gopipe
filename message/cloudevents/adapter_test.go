package cloudevents

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/fxsml/gopipe/message"
)

func TestFromCloudEvent(t *testing.T) {
	t.Run("nil event returns error", func(t *testing.T) {
		_, err := FromCloudEvent(nil, nil)
		if err == nil {
			t.Fatal("expected error for nil event")
		}
	})

	t.Run("converts event to RawMessage", func(t *testing.T) {
		event := cloudevents.NewEvent()
		event.SetID("test-id")
		event.SetType("test.type")
		event.SetSource("/test")
		event.SetSpecVersion("1.0")
		event.SetTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		event.SetDataContentType("application/json")
		event.SetDataSchema("https://example.com/schema")
		event.SetSubject("test-subject")
		if err := event.SetData("application/json", []byte(`{"key":"value"}`)); err != nil {
			t.Fatalf("failed to set data: %v", err)
		}
		event.SetExtension("customext", "custom-value")

		raw, err := FromCloudEvent(&event, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if raw.ID() != "test-id" {
			t.Errorf("expected id 'test-id', got %v", raw.ID())
		}
		if raw.Type() != "test.type" {
			t.Errorf("expected type 'test.type', got %v", raw.Type())
		}
		if raw.Source() != "/test" {
			t.Errorf("expected source '/test', got %v", raw.Source())
		}
		if raw.SpecVersion() != "1.0" {
			t.Errorf("expected specversion '1.0', got %v", raw.SpecVersion())
		}
		if raw.DataContentType() != "application/json" {
			t.Errorf("expected datacontenttype 'application/json', got %v", raw.DataContentType())
		}
		if raw.DataSchema() != "https://example.com/schema" {
			t.Errorf("expected dataschema 'https://example.com/schema', got %v", raw.DataSchema())
		}
		if raw.Subject() != "test-subject" {
			t.Errorf("expected subject 'test-subject', got %v", raw.Subject())
		}
		if raw.Attributes["customext"] != "custom-value" {
			t.Errorf("expected customext 'custom-value', got %v", raw.Attributes["customext"])
		}
		if string(raw.Data) != `{"key":"value"}` {
			t.Errorf("expected data '{\"key\":\"value\"}', got %s", string(raw.Data))
		}
	})

	t.Run("uses provided acking", func(t *testing.T) {
		event := cloudevents.NewEvent()
		event.SetID("test-id")
		event.SetType("test.type")
		event.SetSource("/test")

		acked := false
		acking := message.NewAcking(
			func() { acked = true },
			func(err error) {},
		)

		raw, err := FromCloudEvent(&event, acking)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		raw.Ack()
		if !acked {
			t.Error("expected acking callback to be called")
		}
	})
}

func TestToCloudEvent(t *testing.T) {
	t.Run("nil message returns error", func(t *testing.T) {
		_, err := ToCloudEvent(nil)
		if err == nil {
			t.Fatal("expected error for nil message")
		}
	})

	t.Run("converts RawMessage to event", func(t *testing.T) {
		raw := message.NewRaw(
			[]byte(`{"key":"value"}`),
			message.Attributes{
				"id":              "test-id",
				"type":            "test.type",
				"source":          "/test",
				"specversion":     "1.0",
				"time":            "2025-01-01T00:00:00Z",
				"datacontenttype": "application/json",
				"dataschema":      "https://example.com/schema",
				"subject":         "test-subject",
				"customext":       "custom-value",
			},
			nil,
		)

		event, err := ToCloudEvent(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if event.ID() != "test-id" {
			t.Errorf("expected id 'test-id', got %s", event.ID())
		}
		if event.Type() != "test.type" {
			t.Errorf("expected type 'test.type', got %s", event.Type())
		}
		if event.Source() != "/test" {
			t.Errorf("expected source '/test', got %s", event.Source())
		}
		if event.SpecVersion() != "1.0" {
			t.Errorf("expected specversion '1.0', got %s", event.SpecVersion())
		}
		if event.DataContentType() != "application/json" {
			t.Errorf("expected datacontenttype 'application/json', got %s", event.DataContentType())
		}
		if event.DataSchema() != "https://example.com/schema" {
			t.Errorf("expected dataschema 'https://example.com/schema', got %s", event.DataSchema())
		}
		if event.Subject() != "test-subject" {
			t.Errorf("expected subject 'test-subject', got %s", event.Subject())
		}
		if event.Extensions()["customext"] != "custom-value" {
			t.Errorf("expected customext 'custom-value', got %v", event.Extensions()["customext"])
		}
	})

	t.Run("omits datacontenttype when not in attributes", func(t *testing.T) {
		raw := message.NewRaw(
			[]byte(`{"key":"value"}`),
			message.Attributes{
				"id":     "test-id",
				"type":   "test.type",
				"source": "/test",
			},
			nil,
		)

		event, err := ToCloudEvent(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if event.DataContentType() != "" {
			t.Errorf("expected empty datacontenttype, got %s", event.DataContentType())
		}
	})
}

func TestExtractAttributes(t *testing.T) {
	event := cloudevents.NewEvent()
	event.SetID("test-id")
	event.SetType("test.type")
	event.SetSource("/test")
	event.SetSpecVersion("1.0")
	event.SetTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	event.SetDataContentType("application/json")
	event.SetDataSchema("https://example.com/schema")
	event.SetSubject("test-subject")
	event.SetExtension("customext", "custom-value")

	attrs := extractAttributes(&event)

	if attrs["id"] != "test-id" {
		t.Errorf("expected id 'test-id', got %v", attrs["id"])
	}
	if attrs["type"] != "test.type" {
		t.Errorf("expected type 'test.type', got %v", attrs["type"])
	}
	if attrs["source"] != "/test" {
		t.Errorf("expected source '/test', got %v", attrs["source"])
	}
	if attrs["dataschema"] != "https://example.com/schema" {
		t.Errorf("expected dataschema 'https://example.com/schema', got %v", attrs["dataschema"])
	}
	if attrs["subject"] != "test-subject" {
		t.Errorf("expected subject 'test-subject', got %v", attrs["subject"])
	}
	if attrs["customext"] != "custom-value" {
		t.Errorf("expected customext 'custom-value', got %v", attrs["customext"])
	}
}

func TestSDKCompatibility(t *testing.T) {
	t.Run("RawMessage JSON data roundtrip through SDK", func(t *testing.T) {
		// Create a RawMessage with JSON data
		original := message.NewRaw(
			[]byte(`{"key":"value","count":42}`),
			message.Attributes{
				message.AttrID:     "json-test",
				message.AttrType:   "test.json",
				message.AttrSource: "/test",
			},
			nil,
		)

		// Serialize to JSON using our String() method
		jsonStr := original.String()

		// Parse with SDK
		var sdkEvent cloudevents.Event
		if err := json.Unmarshal([]byte(jsonStr), &sdkEvent); err != nil {
			t.Fatalf("SDK failed to parse our JSON: %v", err)
		}

		// Verify SDK parsed correctly
		if sdkEvent.ID() != "json-test" {
			t.Errorf("SDK id = %s, want json-test", sdkEvent.ID())
		}
		if sdkEvent.Type() != "test.json" {
			t.Errorf("SDK type = %s, want test.json", sdkEvent.Type())
		}

		// Verify data
		var data map[string]any
		if err := sdkEvent.DataAs(&data); err != nil {
			t.Fatalf("SDK DataAs failed: %v", err)
		}
		if data["key"] != "value" {
			t.Errorf("SDK data[key] = %v, want value", data["key"])
		}
	})

	t.Run("RawMessage binary data roundtrip through SDK", func(t *testing.T) {
		// Create a RawMessage with binary data (not valid JSON)
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
		original := message.NewRaw(
			binaryData,
			message.Attributes{
				message.AttrID:     "binary-test",
				message.AttrType:   "test.binary",
				message.AttrSource: "/test",
			},
			nil,
		)

		// Serialize to JSON using our String() method
		jsonStr := original.String()

		// Verify data_base64 is used
		if !strings.Contains(jsonStr, "data_base64") {
			t.Errorf("expected data_base64 field, got: %s", jsonStr)
		}

		// Parse with SDK
		var sdkEvent cloudevents.Event
		if err := json.Unmarshal([]byte(jsonStr), &sdkEvent); err != nil {
			t.Fatalf("SDK failed to parse our JSON: %v", err)
		}

		// Verify SDK parsed correctly
		if sdkEvent.ID() != "binary-test" {
			t.Errorf("SDK id = %s, want binary-test", sdkEvent.ID())
		}

		// Verify binary data (SDK decodes base64 automatically)
		if !bytes.Equal(sdkEvent.Data(), binaryData) {
			t.Errorf("SDK data = %v, want %v", sdkEvent.Data(), binaryData)
		}
	})

	t.Run("SDK Event JSON to RawMessage", func(t *testing.T) {
		// Create SDK event with JSON data
		sdkEvent := cloudevents.NewEvent()
		sdkEvent.SetID("sdk-json-test")
		sdkEvent.SetType("sdk.json")
		sdkEvent.SetSource("/sdk")
		if err := sdkEvent.SetData("application/json", map[string]any{"sdk": true}); err != nil {
			t.Fatalf("SetData failed: %v", err)
		}

		// Serialize SDK event to JSON
		jsonBytes, err := json.Marshal(sdkEvent)
		if err != nil {
			t.Fatalf("SDK marshal failed: %v", err)
		}

		// Parse with our ParseRaw
		parsed, err := message.ParseRaw(bytes.NewReader(jsonBytes))
		if err != nil {
			t.Fatalf("ParseRaw failed: %v", err)
		}

		// Verify our parsing
		if parsed.ID() != "sdk-json-test" {
			t.Errorf("parsed id = %s, want sdk-json-test", parsed.ID())
		}
		if parsed.Type() != "sdk.json" {
			t.Errorf("parsed type = %s, want sdk.json", parsed.Type())
		}

		// Verify data is valid JSON
		if !json.Valid(parsed.Data) {
			t.Errorf("parsed data is not valid JSON: %s", parsed.Data)
		}
	})

	t.Run("SDK Event binary to RawMessage", func(t *testing.T) {
		// Create SDK event with binary data
		binaryData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
		sdkEvent := cloudevents.NewEvent()
		sdkEvent.SetID("sdk-binary-test")
		sdkEvent.SetType("sdk.binary")
		sdkEvent.SetSource("/sdk")
		if err := sdkEvent.SetData("application/octet-stream", binaryData); err != nil {
			t.Fatalf("SetData failed: %v", err)
		}

		// Serialize SDK event to JSON
		jsonBytes, err := json.Marshal(sdkEvent)
		if err != nil {
			t.Fatalf("SDK marshal failed: %v", err)
		}

		// Verify SDK uses data_base64
		if !strings.Contains(string(jsonBytes), "data_base64") {
			t.Errorf("expected SDK to use data_base64, got: %s", jsonBytes)
		}

		// Parse with our ParseRaw
		parsed, err := message.ParseRaw(bytes.NewReader(jsonBytes))
		if err != nil {
			t.Fatalf("ParseRaw failed: %v", err)
		}

		// Verify our parsing
		if parsed.ID() != "sdk-binary-test" {
			t.Errorf("parsed id = %s, want sdk-binary-test", parsed.ID())
		}

		// Verify binary data was decoded
		if !bytes.Equal(parsed.Data, binaryData) {
			t.Errorf("parsed data = %v, want %v", parsed.Data, binaryData)
		}
	})

	t.Run("full roundtrip: RawMessage -> SDK -> RawMessage", func(t *testing.T) {
		original := message.NewRaw(
			[]byte(`{"roundtrip":true}`),
			message.Attributes{
				message.AttrID:              "roundtrip-test",
				message.AttrType:            "test.roundtrip",
				message.AttrSource:          "/roundtrip",
				message.AttrSubject:         "test-subject",
				message.AttrDataContentType: "application/json",
				message.AttrDataSchema:      "https://example.com/schema",
			},
			nil,
		)

		// RawMessage -> SDK Event via ToCloudEvent
		sdkEvent, err := ToCloudEvent(original)
		if err != nil {
			t.Fatalf("ToCloudEvent failed: %v", err)
		}

		// SDK Event -> RawMessage via FromCloudEvent
		restored, err := FromCloudEvent(sdkEvent, nil)
		if err != nil {
			t.Fatalf("FromCloudEvent failed: %v", err)
		}

		// Verify all attributes preserved
		if restored.ID() != original.ID() {
			t.Errorf("id = %s, want %s", restored.ID(), original.ID())
		}
		if restored.Type() != original.Type() {
			t.Errorf("type = %s, want %s", restored.Type(), original.Type())
		}
		if restored.Source() != original.Source() {
			t.Errorf("source = %s, want %s", restored.Source(), original.Source())
		}
		if restored.Subject() != original.Subject() {
			t.Errorf("subject = %s, want %s", restored.Subject(), original.Subject())
		}
		if restored.DataContentType() != original.DataContentType() {
			t.Errorf("datacontenttype = %s, want %s", restored.DataContentType(), original.DataContentType())
		}
		if restored.DataSchema() != original.DataSchema() {
			t.Errorf("dataschema = %s, want %s", restored.DataSchema(), original.DataSchema())
		}

		// Verify data preserved
		if !bytes.Equal(restored.Data, original.Data) {
			t.Errorf("data = %s, want %s", restored.Data, original.Data)
		}
	})

	t.Run("binary data roundtrip through SDK adapters", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
		original := message.NewRaw(
			binaryData,
			message.Attributes{
				message.AttrID:              "binary-roundtrip",
				message.AttrType:            "test.binary",
				message.AttrSource:          "/binary",
				message.AttrDataContentType: "application/octet-stream",
			},
			nil,
		)

		// RawMessage -> SDK Event
		sdkEvent, err := ToCloudEvent(original)
		if err != nil {
			t.Fatalf("ToCloudEvent failed: %v", err)
		}

		// Verify SDK has the binary data
		if !bytes.Equal(sdkEvent.Data(), binaryData) {
			t.Errorf("SDK data = %v, want %v", sdkEvent.Data(), binaryData)
		}

		// SDK Event -> RawMessage
		restored, err := FromCloudEvent(sdkEvent, nil)
		if err != nil {
			t.Fatalf("FromCloudEvent failed: %v", err)
		}

		// Verify binary data preserved
		if !bytes.Equal(restored.Data, binaryData) {
			t.Errorf("restored data = %v, want %v", restored.Data, binaryData)
		}
	})
}
