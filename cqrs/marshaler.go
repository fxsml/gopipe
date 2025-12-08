package cqrs

import (
	"encoding/json"
	"reflect"
)

// Marshaler handles serialization/deserialization of commands and events.
// It also provides type name extraction for routing.
type Marshaler interface {
	// Marshal serializes a value to bytes
	Marshal(v any) ([]byte, error)

	// Unmarshal deserializes bytes into a value
	Unmarshal(data []byte, v any) error

	// Name returns the type name for routing (e.g., "CreateOrder")
	Name(v any) string
}

// JSONMarshaler is a JSON-based implementation of Marshaler.
type JSONMarshaler struct{}

// NewJSONMarshaler creates a new JSON marshaler.
func NewJSONMarshaler() Marshaler {
	return JSONMarshaler{}
}

// Marshal serializes a value to JSON bytes.
func (m JSONMarshaler) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into a value.
func (m JSONMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Name returns the struct type name, stripping pointer indirection.
// Example: CreateOrder{} -> "CreateOrder"
func (m JSONMarshaler) Name(v any) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
