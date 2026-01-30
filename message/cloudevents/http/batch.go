package http

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/fxsml/gopipe/message"
)

// ParseBatch parses a CloudEvents batch (JSON array) from r into RawMessages.
func ParseBatch(r io.Reader) ([]*message.RawMessage, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading batch body: %w", err)
	}

	return ParseBatchBytes(data)
}

// ParseBatchBytes parses a CloudEvents batch (JSON array) from bytes.
func ParseBatchBytes(data []byte) ([]*message.RawMessage, error) {
	var rawEvents []json.RawMessage
	if err := json.Unmarshal(data, &rawEvents); err != nil {
		return nil, fmt.Errorf("parsing batch JSON array: %w", err)
	}

	msgs := make([]*message.RawMessage, 0, len(rawEvents))
	for i, rawEvent := range rawEvents {
		msg, err := parseRawEvent(rawEvent)
		if err != nil {
			return nil, fmt.Errorf("parsing event %d: %w", i, err)
		}
		msgs = append(msgs, msg)
	}

	return msgs, nil
}

// parseRawEvent parses a single CloudEvent from JSON bytes.
func parseRawEvent(data []byte) (*message.RawMessage, error) {
	var ce struct {
		Data       json.RawMessage `json:"data"`
		DataBase64 string          `json:"data_base64"`
	}
	if err := json.Unmarshal(data, &ce); err != nil {
		return nil, fmt.Errorf("parsing CloudEvent JSON: %w", err)
	}

	var attrs message.Attributes
	if err := json.Unmarshal(data, &attrs); err != nil {
		return nil, fmt.Errorf("parsing attributes: %w", err)
	}
	delete(attrs, "data")
	delete(attrs, "data_base64")

	var eventData []byte
	if ce.DataBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(ce.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("decoding data_base64: %w", err)
		}
		eventData = decoded
	} else {
		eventData = ce.Data
	}

	return message.NewRaw(eventData, attrs, nil), nil
}

// MarshalBatch marshals messages to CloudEvents batch format (JSON array).
func MarshalBatch(msgs []*message.RawMessage) ([]byte, error) {
	events := make([]json.RawMessage, len(msgs))
	for i, msg := range msgs {
		b, err := msg.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("marshaling event %d: %w", i, err)
		}
		events[i] = b
	}
	return json.Marshal(events)
}
