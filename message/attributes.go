package message

import "time"

// Context attribute keys aligned with CloudEvents specification v1.0.2.
// See: https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md
const (
	// AttrID is the unique event identifier.
	// CloudEvents: REQUIRED. Identifies the event.
	AttrID = "id"

	// AttrSource identifies the context in which an event happened.
	// CloudEvents: REQUIRED. URI-reference identifying the context.
	// Example: "/orders/api", "urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66"
	AttrSource = "source"

	// AttrSpecVersion is the CloudEvents specification version.
	// CloudEvents: REQUIRED. Always "1.0" for CloudEvents v1.0.x.
	// This is auto-populated by adapters; producers don't need to set it.
	AttrSpecVersion = "specversion"

	// AttrType describes what kind of event/command occurred.
	// CloudEvents: REQUIRED. Contains a value describing the type of event.
	// Examples: "OrderCreated", "CreateOrder", "com.example.order.created"
	// Used for routing and handler matching. Should NOT be generic values like "event" or "command".
	AttrType = "type"

	// AttrSubject identifies the specific entity this message is about within the source.
	// CloudEvents: OPTIONAL. The subject of the event in the context of the event producer.
	// Example: "order/ORD-001" for an order-related event, "user/123" for a user event.
	AttrSubject = "subject"

	// AttrTime stores when the event occurred.
	// CloudEvents: OPTIONAL. Timestamp of when the occurrence happened.
	AttrTime = "time"

	// AttrDataContentType indicates the content type of the data value.
	// CloudEvents: OPTIONAL. Content type of the data attribute.
	// Example: "application/json", "text/xml"
	AttrDataContentType = "datacontenttype"

	// AttrCorrelationID is used to correlate related messages across services.
	// gopipe extension: not part of CloudEvents core spec.
	AttrCorrelationID = "correlationid"

	// AttrDeadline stores the message processing deadline.
	// gopipe extension: not part of CloudEvents core spec.
	AttrDeadline = "deadline"

	// AttrTopic is the pub/sub topic for routing messages in publish-subscribe systems.
	// gopipe extension: not part of CloudEvents core spec.
	// This attribute is used by publishers to determine message routing and should be set
	// by AttributeProviders or marshalers. Empty string is a valid topic value representing
	// the default topic. Senders should not forward this attribute to the underlying broker.
	AttrTopic = "topic"
)

// String retrieves a string attribute by key.
func (a Attributes) String(key string) (string, bool) {
	if v, ok := a[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// Time retrieves a time.Time attribute by key.
func (a Attributes) Time(key string) (time.Time, bool) {
	if v, ok := a[key]; ok {
		if t, ok := v.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// ID returns the event ID as string from attributes.
func (a Attributes) ID() (string, bool) {
	return a.String(AttrID)
}

// Source returns the event source URI as string from attributes.
func (a Attributes) Source() (string, bool) {
	return a.String(AttrSource)
}

// SpecVersion returns the CloudEvents spec version as string from attributes.
func (a Attributes) SpecVersion() (string, bool) {
	return a.String(AttrSpecVersion)
}

// Type returns the event type as string from attributes.
func (a Attributes) Type() (string, bool) {
	return a.String(AttrType)
}

// Subject returns the subject as string from attributes.
func (a Attributes) Subject() (string, bool) {
	return a.String(AttrSubject)
}

// EventTime returns the event timestamp from attributes.
func (a Attributes) EventTime() (time.Time, bool) {
	return a.Time(AttrTime)
}

// DataContentType returns the data content type as string from attributes.
func (a Attributes) DataContentType() (string, bool) {
	return a.String(AttrDataContentType)
}

// CorrelationID returns the correlation ID as string from attributes.
func (a Attributes) CorrelationID() (string, bool) {
	return a.String(AttrCorrelationID)
}

// Deadline returns the deadline for processing this message.
func (a Attributes) Deadline() (time.Time, bool) {
	return a.Time(AttrDeadline)
}

// Topic returns the pub/sub topic as string from attributes.
// Empty string is a valid topic representing the default topic.
func (a Attributes) Topic() (string, bool) {
	return a.String(AttrTopic)
}
