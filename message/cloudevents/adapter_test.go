package cloudevents

import (
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

		if raw.Attributes["id"] != "test-id" {
			t.Errorf("expected id 'test-id', got %v", raw.Attributes["id"])
		}
		if raw.Attributes["type"] != "test.type" {
			t.Errorf("expected type 'test.type', got %v", raw.Attributes["type"])
		}
		if raw.Attributes["source"] != "/test" {
			t.Errorf("expected source '/test', got %v", raw.Attributes["source"])
		}
		if raw.Attributes["specversion"] != "1.0" {
			t.Errorf("expected specversion '1.0', got %v", raw.Attributes["specversion"])
		}
		if raw.Attributes["datacontenttype"] != "application/json" {
			t.Errorf("expected datacontenttype 'application/json', got %v", raw.Attributes["datacontenttype"])
		}
		if raw.Attributes["dataschema"] != "https://example.com/schema" {
			t.Errorf("expected dataschema 'https://example.com/schema', got %v", raw.Attributes["dataschema"])
		}
		if raw.Attributes["subject"] != "test-subject" {
			t.Errorf("expected subject 'test-subject', got %v", raw.Attributes["subject"])
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
