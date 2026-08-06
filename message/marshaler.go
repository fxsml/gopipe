package message

// Marshaler handles serialization and deserialization of message data
// at the broker boundary, where Data is always typed []byte (see Message.Raw).
type Marshaler interface {
	// Marshal encodes v to bytes. v may be nil; implementations must not
	// panic — how nil is represented, or whether it's rejected, is
	// implementation-defined.
	Marshal(v any) ([]byte, error)

	// Unmarshal decodes data into v. v must be a non-nil pointer; if not,
	// Unmarshal returns an error rather than panicking. Handling of empty
	// data (len(data) == 0) is implementation-defined but must be
	// deterministic.
	Unmarshal(data []byte, v any) error

	// DataContentType returns the CloudEvents datacontenttype attribute value.
	// Example: "application/json"
	DataContentType() string
}
