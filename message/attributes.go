package message

// Attributes is a map of message context attributes per CloudEvents spec.
// CloudEvents defines attributes as the metadata that describes the event.
//
// Thread safety: Attributes is not safe for concurrent read/write access.
// Handlers receive a single message at a time, so concurrent access is rare.
// If sharing attributes between goroutines, use external synchronization.
type Attributes map[string]any

// CloudEvents attribute keys for use in Attributes map literals.
const (
	// AttrID is required by CloudEvents. Unique event identifier.
	AttrID = "id"
	// AttrType is required by CloudEvents. Event type (e.g., "order.created").
	AttrType = "type"
	// AttrSource is required by CloudEvents. Event source URI.
	AttrSource = "source"
	// AttrSpecVersion is required by CloudEvents. Spec version (default "1.0").
	AttrSpecVersion = "specversion"
	// AttrSubject is optional in CloudEvents. Event subject/context.
	AttrSubject = "subject"
	// AttrTime is optional in CloudEvents. Event timestamp (RFC3339).
	AttrTime = "time"
	// AttrDataContentType is optional in CloudEvents. Data content type.
	AttrDataContentType = "datacontenttype"
	// AttrDataSchema is optional in CloudEvents. Data schema URI.
	AttrDataSchema = "dataschema"
)
