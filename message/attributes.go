package message

import "github.com/google/uuid"

// NewID generates a new unique message ID (UUID v4).
func NewID() string { return uuid.NewString() }

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

// CloudEvents extension attribute keys.
// Extensions are custom attributes not defined in the core CloudEvents spec.
// Extension names follow CloudEvents naming rules: lowercase a-z, 0-9 only, max 20 chars.
const (
	// AttrCorrelationID correlates related events. Propagated by middleware.CorrelationID.
	AttrCorrelationID = "correlationid"

	// AttrExpiryTime is the event expiration timestamp. Used by middleware.Deadline.
	AttrExpiryTime = "expirytime"
)

// requiredAttrs contains CloudEvents required context attributes per CloudEvents 1.0 spec.
var requiredAttrs = map[string]struct{}{
	AttrID:          {},
	AttrType:        {},
	AttrSource:      {},
	AttrSpecVersion: {},
}

// optionalAttrs contains CloudEvents optional context attributes per CloudEvents 1.0 spec.
var optionalAttrs = map[string]struct{}{
	AttrSubject:         {},
	AttrTime:            {},
	AttrDataContentType: {},
	AttrDataSchema:      {},
}

// IsRequiredAttr returns true if key is a required CloudEvents context attribute
// per CloudEvents 1.0 spec: id, source, specversion, type.
func IsRequiredAttr(key string) bool {
	_, ok := requiredAttrs[key]
	return ok
}

// IsOptionalAttr returns true if key is an optional CloudEvents context attribute
// per CloudEvents 1.0 spec: subject, time, datacontenttype, dataschema.
func IsOptionalAttr(key string) bool {
	_, ok := optionalAttrs[key]
	return ok
}

// IsExtensionAttr returns true if key is a CloudEvents extension attribute
// (not defined in the core CloudEvents spec). Examples: correlationid, expirytime.
func IsExtensionAttr(key string) bool {
	return !IsRequiredAttr(key) && !IsOptionalAttr(key)
}
