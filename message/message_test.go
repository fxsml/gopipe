package message

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("creates message with data and attributes", func(t *testing.T) {
		data := "test data"
		attrs := Attributes{"key": "value"}
		msg := New(data, attrs, nil)

		if msg.Data != data {
			t.Errorf("expected data %q, got %q", data, msg.Data)
		}
		if msg.Attributes["key"] != "value" {
			t.Errorf("expected attribute key=value, got %v", msg.Attributes["key"])
		}
	})

	t.Run("creates empty attributes when nil", func(t *testing.T) {
		msg := New("data", nil, nil)
		if msg.Attributes == nil {
			t.Error("expected non-nil attributes map")
		}
	})

	t.Run("ack callback is invoked", func(t *testing.T) {
		acked := false
		msg := New("data", nil, NewAcking(func() { acked = true }, func(error) {}))

		msg.Ack()
		if !acked {
			t.Error("expected ack callback to be invoked")
		}
	})

	t.Run("nack callback is invoked with error", func(t *testing.T) {
		var nackErr error
		msg := New("data", nil, NewAcking(func() {}, func(err error) { nackErr = err }))

		testErr := ErrNoHandler
		msg.Nack(testErr)
		if nackErr != testErr {
			t.Errorf("expected nack error %v, got %v", testErr, nackErr)
		}
	})
}

func TestNewTyped(t *testing.T) {
	t.Run("creates typed message", func(t *testing.T) {
		msg := NewTyped("hello", Attributes{"key": "value"}, nil)
		if msg.Data != "hello" {
			t.Errorf("expected data hello, got %v", msg.Data)
		}
		if msg.Attributes["key"] != "value" {
			t.Errorf("expected attribute key=value, got %v", msg.Attributes["key"])
		}
	})

	t.Run("creates empty attributes when nil", func(t *testing.T) {
		msg := NewTyped(123, nil, nil)
		if msg.Attributes == nil {
			t.Error("expected non-nil attributes map")
		}
	})
}

func TestNewRaw(t *testing.T) {
	t.Run("creates raw message", func(t *testing.T) {
		data := []byte(`{"id":123}`)
		msg := NewRaw(data, Attributes{"type": "test"}, nil)
		if string(msg.Data) != string(data) {
			t.Errorf("expected data %s, got %s", data, msg.Data)
		}
		if msg.Type() != "test" {
			t.Errorf("expected type test, got %v", msg.Type())
		}
	})

	t.Run("with acking for broker integration", func(t *testing.T) {
		acked := false
		msg := NewRaw([]byte("data"), nil, NewAcking(func() { acked = true }, func(error) {}))

		msg.Ack()
		if !acked {
			t.Error("expected ack callback to be invoked")
		}
	})
}

func TestAcking(t *testing.T) {
	t.Run("ack requires expected count", func(t *testing.T) {
		acked := false
		acking := NewSharedAcking(func() { acked = true }, func(error) {}, 3)

		msg1 := New("data1", nil, acking)
		msg2 := New("data2", nil, acking)
		msg3 := New("data3", nil, acking)

		msg1.Ack()
		if acked {
			t.Error("acked too early after 1 ack")
		}

		msg2.Ack()
		if acked {
			t.Error("acked too early after 2 acks")
		}

		msg3.Ack()
		if !acked {
			t.Error("expected ack after 3 acks")
		}
	})

	t.Run("nack blocks further acks", func(t *testing.T) {
		acked := false
		nacked := false
		acking := NewSharedAcking(func() { acked = true }, func(error) { nacked = true }, 2)

		msg1 := New("data1", nil, acking)
		msg2 := New("data2", nil, acking)

		msg1.Nack(ErrNoHandler)
		if !nacked {
			t.Error("expected nack callback")
		}

		result := msg2.Ack()
		if result {
			t.Error("expected Ack to return false after Nack")
		}
		if acked {
			t.Error("ack callback should not be invoked after nack")
		}
	})

	t.Run("ack is idempotent", func(t *testing.T) {
		count := 0
		msg := New("data", nil, NewAcking(func() { count++ }, func(error) {}))

		msg.Ack()
		msg.Ack()
		msg.Ack()

		if count != 1 {
			t.Errorf("expected ack callback once, got %d", count)
		}
	})

	t.Run("nack is idempotent", func(t *testing.T) {
		count := 0
		msg := New("data", nil, NewAcking(func() {}, func(error) { count++ }))

		msg.Nack(ErrNoHandler)
		msg.Nack(ErrNoHandler)

		if count != 1 {
			t.Errorf("expected nack callback once, got %d", count)
		}
	})

	t.Run("thread safety", func(t *testing.T) {
		acked := false
		acking := NewSharedAcking(func() { acked = true }, func(error) {}, 100)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				msg := New("data", nil, acking)
				msg.Ack()
			}()
		}
		wg.Wait()

		if !acked {
			t.Error("expected ack after 100 concurrent acks")
		}
	})

	t.Run("nil acking returns false", func(t *testing.T) {
		msg := NewTyped[string]("data", nil, nil)
		if msg.Ack() {
			t.Error("expected Ack to return false with nil acking")
		}
		if msg.Nack(ErrNoHandler) {
			t.Error("expected Nack to return false with nil acking")
		}
	})

	t.Run("NewAcking returns nil for invalid params", func(t *testing.T) {
		if NewAcking(nil, func(error) {}) != nil {
			t.Error("expected nil for nil ack")
		}
		if NewAcking(func() {}, nil) != nil {
			t.Error("expected nil for nil nack")
		}
	})

	t.Run("NewSharedAcking returns nil for invalid params", func(t *testing.T) {
		if NewSharedAcking(nil, func(error) {}, 2) != nil {
			t.Error("expected nil for nil ack")
		}
		if NewSharedAcking(func() {}, nil, 2) != nil {
			t.Error("expected nil for nil nack")
		}
		if NewSharedAcking(func() {}, func(error) {}, 0) != nil {
			t.Error("expected nil for zero count")
		}
		if NewSharedAcking(func() {}, func(error) {}, -1) != nil {
			t.Error("expected nil for negative count")
		}
	})
}

func TestCopy(t *testing.T) {
	t.Run("preserves attributes and acking", func(t *testing.T) {
		acked := false
		original := New("original", Attributes{"key": "value"}, NewAcking(func() { acked = true }, func(error) {}))

		copied := Copy(original, "copied")

		if copied.Data != "copied" {
			t.Errorf("expected copied data, got %v", copied.Data)
		}
		if copied.Attributes["key"] != "value" {
			t.Error("expected attributes to be preserved")
		}

		copied.Ack()
		if !acked {
			t.Error("expected shared acking to work")
		}
	})
}

func TestString(t *testing.T) {
	t.Run("injects specversion when missing", func(t *testing.T) {
		msg := New("hello", Attributes{"type": "greeting"}, nil)
		s := msg.String()

		var ce map[string]any
		if err := json.Unmarshal([]byte(s), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if ce["specversion"] != "1.0" {
			t.Errorf("expected specversion 1.0, got %v", ce["specversion"])
		}
		if ce["type"] != "greeting" {
			t.Errorf("expected type greeting, got %v", ce["type"])
		}
		if ce["data"] != "hello" {
			t.Errorf("expected data hello, got %v", ce["data"])
		}
	})

	t.Run("preserves existing specversion", func(t *testing.T) {
		msg := New("data", Attributes{"specversion": "2.0"}, nil)
		s := msg.String()

		var ce map[string]any
		if err := json.Unmarshal([]byte(s), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if ce["specversion"] != "2.0" {
			t.Errorf("expected specversion 2.0, got %v", ce["specversion"])
		}
	})

	t.Run("embeds valid JSON bytes as raw JSON", func(t *testing.T) {
		data := []byte(`{"orderId":"123","amount":50}`)
		msg := New(data, Attributes{"type": "order.created"}, nil)
		s := msg.String()

		// Should contain the raw JSON, not base64 encoded
		if !strings.Contains(s, `"orderId"`) {
			t.Errorf("expected raw JSON in output, got %s", s)
		}

		var ce map[string]any
		if err := json.Unmarshal([]byte(s), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		dataMap, ok := ce["data"].(map[string]any)
		if !ok {
			t.Fatalf("expected data to be object, got %T", ce["data"])
		}
		if dataMap["orderId"] != "123" {
			t.Errorf("expected orderId 123, got %v", dataMap["orderId"])
		}
	})

	t.Run("handles invalid JSON bytes", func(t *testing.T) {
		data := []byte("not json")
		msg := New(data, nil, nil)
		s := msg.String()

		// Should still produce valid JSON output
		var ce map[string]any
		if err := json.Unmarshal([]byte(s), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
	})
}

func TestWriteTo(t *testing.T) {
	t.Run("writes to buffer", func(t *testing.T) {
		msg := New("hello", Attributes{"type": "greeting"}, nil)
		var buf bytes.Buffer

		n, err := msg.WriteTo(&buf)
		if err != nil {
			t.Fatalf("WriteTo failed: %v", err)
		}
		if n != int64(buf.Len()) {
			t.Errorf("expected n=%d, got %d", buf.Len(), n)
		}

		var ce map[string]any
		if err := json.Unmarshal(buf.Bytes(), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if ce["specversion"] != "1.0" {
			t.Errorf("expected specversion 1.0, got %v", ce["specversion"])
		}
		if ce["data"] != "hello" {
			t.Errorf("expected data hello, got %v", ce["data"])
		}
	})

	t.Run("embeds raw JSON for byte slices", func(t *testing.T) {
		data := []byte(`{"id":123}`)
		msg := New(data, nil, nil)
		var buf bytes.Buffer

		_, err := msg.WriteTo(&buf)
		if err != nil {
			t.Fatalf("WriteTo failed: %v", err)
		}

		var ce map[string]any
		if err := json.Unmarshal(buf.Bytes(), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		dataMap, ok := ce["data"].(map[string]any)
		if !ok {
			t.Fatalf("expected data to be object, got %T", ce["data"])
		}
		if dataMap["id"] != float64(123) {
			t.Errorf("expected id 123, got %v", dataMap["id"])
		}
	})
}

func TestParseRaw(t *testing.T) {
	t.Run("parses CloudEvents JSON", func(t *testing.T) {
		input := `{"specversion":"1.0","type":"order.created","source":"/test","id":"123","data":{"order_id":"ABC"}}`
		msg, err := ParseRaw(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseRaw failed: %v", err)
		}

		if msg.Type() != "order.created" {
			t.Errorf("expected type order.created, got %v", msg.Type())
		}
		if msg.SpecVersion() != "1.0" {
			t.Errorf("expected specversion 1.0, got %v", msg.SpecVersion())
		}
		if string(msg.Data) != `{"order_id":"ABC"}` {
			t.Errorf("expected data {\"order_id\":\"ABC\"}, got %s", msg.Data)
		}
	})

	t.Run("roundtrip with WriteTo", func(t *testing.T) {
		original := NewRaw([]byte(`{"id":456}`), Attributes{
			"type":   "test.event",
			"source": "/roundtrip",
		}, nil)

		var buf bytes.Buffer
		_, _ = original.WriteTo(&buf)

		parsed, err := ParseRaw(&buf)
		if err != nil {
			t.Fatalf("ParseRaw failed: %v", err)
		}

		if parsed.Type() != "test.event" {
			t.Errorf("expected type test.event, got %v", parsed.Type())
		}
		if string(parsed.Data) != `{"id":456}` {
			t.Errorf("expected data {\"id\":456}, got %s", parsed.Data)
		}
	})

	t.Run("parses data_base64 field", func(t *testing.T) {
		// "hello world" base64 encoded
		input := `{"specversion":"1.0","type":"binary.event","source":"/test","id":"123","data_base64":"aGVsbG8gd29ybGQ="}`
		msg, err := ParseRaw(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseRaw failed: %v", err)
		}

		if msg.Type() != "binary.event" {
			t.Errorf("expected type binary.event, got %v", msg.Type())
		}
		if string(msg.Data) != "hello world" {
			t.Errorf("expected data 'hello world', got %s", msg.Data)
		}
	})

	t.Run("roundtrip with binary data", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
		original := NewRaw(binaryData, Attributes{
			"type":   "binary.event",
			"source": "/binary",
		}, nil)

		var buf bytes.Buffer
		_, _ = original.WriteTo(&buf)

		// Verify data_base64 is used in JSON output
		jsonStr := buf.String()
		if !strings.Contains(jsonStr, "data_base64") {
			t.Errorf("expected data_base64 field in JSON, got: %s", jsonStr)
		}
		if strings.Contains(jsonStr, `"data":`) {
			t.Errorf("unexpected data field in JSON for binary data, got: %s", jsonStr)
		}

		parsed, err := ParseRaw(strings.NewReader(jsonStr))
		if err != nil {
			t.Fatalf("ParseRaw failed: %v", err)
		}

		if !bytes.Equal(parsed.Data, binaryData) {
			t.Errorf("expected data %v, got %v", binaryData, parsed.Data)
		}
	})
}

func TestTime(t *testing.T) {
	t.Run("returns time.Time value", func(t *testing.T) {
		ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
		msg := New("data", Attributes{AttrTime: ts}, nil)

		got := msg.Time()
		if !got.Equal(ts) {
			t.Errorf("expected %v, got %v", ts, got)
		}
	})

	t.Run("parses RFC3339 string", func(t *testing.T) {
		ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
		msg := New("data", Attributes{AttrTime: "2025-01-15T10:30:00Z"}, nil)

		got := msg.Time()
		if !got.Equal(ts) {
			t.Errorf("expected %v, got %v", ts, got)
		}
	})

	t.Run("returns zero time when missing", func(t *testing.T) {
		msg := New("data", nil, nil)

		got := msg.Time()
		if !got.IsZero() {
			t.Errorf("expected zero time, got %v", got)
		}
	})

	t.Run("returns zero time for invalid type", func(t *testing.T) {
		msg := New("data", Attributes{AttrTime: 12345}, nil)

		got := msg.Time()
		if !got.IsZero() {
			t.Errorf("expected zero time, got %v", got)
		}
	})

	t.Run("serializes time.Time to RFC3339", func(t *testing.T) {
		ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
		msg := New("data", Attributes{AttrTime: ts, AttrType: "test"}, nil)
		s := msg.String()

		var ce map[string]any
		if err := json.Unmarshal([]byte(s), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		timeStr, ok := ce["time"].(string)
		if !ok {
			t.Fatalf("expected time as string, got %T", ce["time"])
		}
		if timeStr != "2025-01-15T10:30:00Z" {
			t.Errorf("expected 2025-01-15T10:30:00Z, got %s", timeStr)
		}
	})

	t.Run("preserves RFC3339 string in serialization", func(t *testing.T) {
		msg := New("data", Attributes{AttrTime: "2025-01-15T10:30:00Z", AttrType: "test"}, nil)
		s := msg.String()

		var ce map[string]any
		if err := json.Unmarshal([]byte(s), &ce); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		timeStr, ok := ce["time"].(string)
		if !ok {
			t.Fatalf("expected time as string, got %T", ce["time"])
		}
		if timeStr != "2025-01-15T10:30:00Z" {
			t.Errorf("expected 2025-01-15T10:30:00Z, got %s", timeStr)
		}
	})
}
