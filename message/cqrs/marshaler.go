package cqrs

import (
	"encoding/json"

	"github.com/fxsml/gopipe/message"
)

// Marshaler handles serialization of messages.
type Marshaler interface {
	// Marshal serializes a value to bytes
	Marshal(v any) ([]byte, error)

	// Attributes returns message attributes for the given value
	Attributes(v any) message.Attributes
}

// Unmarshaler handles deserialization of messages.
type Unmarshaler interface {
	// Unmarshal deserializes bytes into a value
	Unmarshal(data []byte, v any) error
}

// CommandMarshaler handles serialization and attribute transformation for commands and events.
type CommandMarshaler interface {
	Marshaler
	Unmarshaler
}

// EventMarshaler handles deserialization of events for event handlers.
type EventMarshaler interface {
	Unmarshaler
}

// ============================================================================
// JSON Implementations
// ============================================================================

// JSONCommandMarshaler is a JSON-based implementation of CommandMarshaler.
type JSONCommandMarshaler struct {
	attrProviders []AttributeProvider
}

// NewJSONCommandMarshaler creates a JSON marshaler with attribute providers.
// Defaults to WithType() if no providers specified.
func NewJSONCommandMarshaler(providers ...AttributeProvider) *JSONCommandMarshaler {
	if len(providers) == 0 {
		providers = []AttributeProvider{WithTypeOf()}
	}
	return &JSONCommandMarshaler{attrProviders: providers}
}

// Marshal serializes a value to JSON bytes.
func (m *JSONCommandMarshaler) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into a value.
func (m *JSONCommandMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Attributes returns message attributes by applying all configured attribute providers.
func (m *JSONCommandMarshaler) Attributes(v any) message.Attributes {
	result := make(message.Attributes)
	for _, provider := range m.attrProviders {
		for k, val := range provider(v) {
			result[k] = val
		}
	}
	return result
}

// JSONEventMarshaler is a JSON-based implementation of EventMarshaler.
type JSONEventMarshaler struct{}

// NewJSONEventMarshaler creates a new JSON event marshaler.
func NewJSONEventMarshaler() *JSONEventMarshaler {
	return &JSONEventMarshaler{}
}

// Unmarshal deserializes JSON bytes into a value.
func (m *JSONEventMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
