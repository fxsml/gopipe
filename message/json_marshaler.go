package message

import "encoding/json"

// JSONMarshaler implements Marshaler using JSON encoding.
type JSONMarshaler struct{}

// NewJSONMarshaler creates a new JSON marshaler.
func NewJSONMarshaler() *JSONMarshaler {
	return &JSONMarshaler{}
}

// Marshal encodes a value to JSON bytes.
func (m *JSONMarshaler) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal decodes JSON bytes into a value.
func (m *JSONMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// DataContentType returns "application/json".
func (m *JSONMarshaler) DataContentType() string {
	return "application/json"
}
