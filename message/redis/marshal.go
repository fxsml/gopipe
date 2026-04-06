package redis

import (
	"encoding/json"
	"fmt"

	"github.com/fxsml/gopipe/message"
)

// Stream entry field keys.
const (
	fieldData       = "data"
	fieldAttributes = "attributes"
)

// MarshalFunc converts a RawMessage to Redis Stream entry field-value pairs.
type MarshalFunc func(msg *message.RawMessage) (map[string]any, error)

// UnmarshalFunc converts Redis Stream entry field-value pairs to a RawMessage.
// The id parameter is the stream entry ID (e.g., "1234567890-0").
type UnmarshalFunc func(id string, values map[string]any) (*message.RawMessage, error)

// DefaultMarshal serializes a RawMessage as JSON fields for a Redis Stream entry.
func DefaultMarshal(msg *message.RawMessage) (map[string]any, error) {
	attrs, err := json.Marshal(msg.Attributes)
	if err != nil {
		return nil, fmt.Errorf("marshal attributes: %w", err)
	}
	return map[string]any{
		fieldData:       string(msg.Data),
		fieldAttributes: string(attrs),
	}, nil
}

// DefaultUnmarshal deserializes a Redis Stream entry into a RawMessage.
// Acking is not set — the caller must attach it after unmarshaling.
func DefaultUnmarshal(_ string, values map[string]any) (*message.RawMessage, error) {
	data, ok := values[fieldData]
	if !ok {
		return nil, fmt.Errorf("missing %q field in stream entry", fieldData)
	}

	var dataBytes []byte
	switch v := data.(type) {
	case string:
		dataBytes = []byte(v)
	case []byte:
		dataBytes = v
	default:
		return nil, fmt.Errorf("unexpected type for %q field: %T", fieldData, data)
	}

	var attrs message.Attributes
	if raw, ok := values[fieldAttributes]; ok {
		var attrBytes []byte
		switch v := raw.(type) {
		case string:
			attrBytes = []byte(v)
		case []byte:
			attrBytes = v
		default:
			return nil, fmt.Errorf("unexpected type for %q field: %T", fieldAttributes, raw)
		}
		if err := json.Unmarshal(attrBytes, &attrs); err != nil {
			return nil, fmt.Errorf("unmarshal attributes: %w", err)
		}
	}

	return message.NewRaw(dataBytes, attrs, nil), nil
}
