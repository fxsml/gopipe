package message

import "fmt"

// DataValidator validates raw data bytes.
// The instance v is the Go type being marshaled or unmarshaled.
// It can be used for type-based schema lookup (e.g., via reflect.TypeOf).
type DataValidator func(data []byte, v any) error

// ValidatingMarshalerConfig configures a ValidatingMarshaler.
type ValidatingMarshalerConfig struct {
	// UnmarshalValidation validates raw data before unmarshaling.
	// If nil, no pre-unmarshal validation is performed.
	UnmarshalValidation DataValidator

	// MarshalValidation validates serialized data after marshaling.
	// If nil, no post-marshal validation is performed.
	MarshalValidation DataValidator
}

// ValidatingMarshaler wraps a Marshaler with optional data validation.
// Validation runs before unmarshal and after marshal, allowing schema or
// custom validation without changing the underlying serialization.
type ValidatingMarshaler struct {
	inner Marshaler
	cfg   ValidatingMarshalerConfig
}

// NewValidatingMarshaler creates a marshaler that decorates inner with validation.
func NewValidatingMarshaler(inner Marshaler, cfg ValidatingMarshalerConfig) *ValidatingMarshaler {
	return &ValidatingMarshaler{inner: inner, cfg: cfg}
}

// Marshal encodes a value to bytes, then validates the result.
func (m *ValidatingMarshaler) Marshal(v any) ([]byte, error) {
	data, err := m.inner.Marshal(v)
	if err != nil {
		return nil, err
	}

	if m.cfg.MarshalValidation != nil {
		if err := m.cfg.MarshalValidation(data, v); err != nil {
			return nil, fmt.Errorf("marshal validation: %w", err)
		}
	}

	return data, nil
}

// Unmarshal validates raw data, then decodes it into v.
func (m *ValidatingMarshaler) Unmarshal(data []byte, v any) error {
	if m.cfg.UnmarshalValidation != nil {
		if err := m.cfg.UnmarshalValidation(data, v); err != nil {
			return fmt.Errorf("unmarshal validation: %w", err)
		}
	}

	return m.inner.Unmarshal(data, v)
}

// DataContentType delegates to the inner marshaler.
func (m *ValidatingMarshaler) DataContentType() string {
	return m.inner.DataContentType()
}
