package message

// Marshaler handles serialization and deserialization of message data.
type Marshaler interface {
	// Marshal encodes a value to bytes.
	Marshal(v any) ([]byte, error)

	// Unmarshal decodes bytes into a value.
	Unmarshal(data []byte, v any) error

	// DataContentType returns the CloudEvents datacontenttype attribute value.
	// Example: "application/json"
	DataContentType() string
}
