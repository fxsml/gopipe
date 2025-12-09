package cqrs

import (
	"encoding/json"

	"github.com/fxsml/gopipe/message"
)

// CommandMarshaler handles serialization/deserialization of commands and events,
// and provides property transformation for output messages.
//
// This is used by NewCommandHandler to marshal commands (input) and events (output).
type CommandMarshaler interface {
	// Marshal serializes a value to bytes
	Marshal(v any) ([]byte, error)

	// Unmarshal deserializes bytes into a value
	Unmarshal(data []byte, v any) error

	// Props returns message properties for the given value.
	// This replaces the old props parameter in NewCommandHandler.
	// For correlation ID propagation, use message-level middleware instead.
	Props(v any) message.Properties
}

// EventMarshaler handles deserialization of events.
//
// This is used by NewEventHandler to unmarshal events (input only).
// Event handlers don't produce output messages, so only Unmarshal is needed.
type EventMarshaler interface {
	// Unmarshal deserializes bytes into a value
	Unmarshal(data []byte, v any) error
}

// ============================================================================
// JSON Implementations
// ============================================================================

// JSONCommandMarshaler is a JSON-based implementation of CommandMarshaler.
type JSONCommandMarshaler struct {
	propProviders []PropertyProvider
}

// NewJSONCommandMarshaler creates a new JSON command marshaler with property providers.
//
// If no providers are specified, it uses sensible defaults:
// - WithTypeName(): Set PropType to the reflected type name
//
// For correlation ID propagation, use message-level middleware instead.
//
// Example with custom providers:
//
//	marshaler := NewJSONCommandMarshaler(
//	    WithType("event"),
//	    WithTypeName(),
//	)
func NewJSONCommandMarshaler(providers ...PropertyProvider) *JSONCommandMarshaler {
	// Use default providers if none specified
	if len(providers) == 0 {
		providers = []PropertyProvider{
			WithTypeName(),
		}
	}
	return &JSONCommandMarshaler{propProviders: providers}
}

// Marshal serializes a value to JSON bytes.
func (m *JSONCommandMarshaler) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into a value.
func (m *JSONCommandMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Props returns message properties by applying all configured property providers.
func (m *JSONCommandMarshaler) Props(v any) message.Properties {
	result := make(message.Properties)
	for _, provider := range m.propProviders {
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

// ============================================================================
// Backward Compatibility
// ============================================================================

// Marshaler is the old interface kept for backward compatibility.
// Deprecated: Use CommandMarshaler or EventMarshaler instead.
type Marshaler = CommandMarshaler

// JSONMarshaler is the old implementation kept for backward compatibility.
// Deprecated: Use JSONCommandMarshaler or JSONEventMarshaler instead.
type JSONMarshaler = JSONCommandMarshaler

// NewJSONMarshaler creates a new JSON marshaler (backward compatible).
// Deprecated: Use NewJSONCommandMarshaler or NewJSONEventMarshaler instead.
func NewJSONMarshaler() CommandMarshaler {
	return NewJSONCommandMarshaler()
}
