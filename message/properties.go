package message

import "time"

// Reserved property keys for standard message metadata.
// Property keys are aligned with CloudEvents specification v1.0.2 where applicable.
const (
	// PropID is the unique message identifier.
	// CloudEvents: REQUIRED. Identifies the event.
	PropID = "id"

	// PropSource identifies the context in which an event happened.
	// CloudEvents: REQUIRED. URI-reference identifying the context.
	// Example: "/orders/api", "urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66"
	PropSource = "source"

	// PropSpecVersion is the CloudEvents specification version.
	// CloudEvents: REQUIRED. Always "1.0" for CloudEvents v1.0.x.
	// This is auto-populated by adapters; producers don't need to set it.
	PropSpecVersion = "specversion"

	// PropType describes what kind of event/command occurred.
	// CloudEvents: REQUIRED. Contains a value describing the type of event.
	// Examples: "OrderCreated", "CreateOrder", "com.example.order.created"
	// Used for routing and handler matching. Should NOT be generic values like "event" or "command".
	PropType = "type"

	// PropSubject identifies the specific entity this message is about within the source.
	// CloudEvents: OPTIONAL. The subject of the event in the context of the event producer.
	// Example: "order/ORD-001" for an order-related event, "user/123" for a user event.
	PropSubject = "subject"

	// PropTime stores when the message was created.
	// CloudEvents: OPTIONAL. Timestamp of when the occurrence happened.
	PropTime = "time"

	// PropDataContentType indicates the content type of the data value.
	// CloudEvents: OPTIONAL. Content type of the data attribute.
	// Example: "application/json", "text/xml"
	PropDataContentType = "datacontenttype"

	// PropCorrelationID is used to correlate related messages across services.
	// gopipe extension: not part of CloudEvents core spec.
	PropCorrelationID = "correlationid"

	// PropDeadline stores the message processing deadline.
	// gopipe extension: not part of CloudEvents core spec.
	PropDeadline = "deadline"

	// PropTopic is the pub/sub topic for routing messages in publish-subscribe systems.
	// gopipe extension: not part of CloudEvents core spec.
	// This property is used by publishers to determine message routing and should be set
	// by PropertyProviders or marshalers. Empty string is a valid topic value representing
	// the default topic. Senders should not forward this property to the underlying broker.
	PropTopic = "topic"
)

// Deprecated property constants for backwards compatibility.
// These will be removed in a future version.
const (
	// Deprecated: Use PropTime instead.
	PropCreatedAt = PropTime

	// Deprecated: Use PropDataContentType instead.
	PropContentType = PropDataContentType
)

// String retrieves a string property by key.
func (p Properties) String(key string) (string, bool) {
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// Time retrieves a time.Time property by key.
func (p Properties) Time(key string) (time.Time, bool) {
	if v, ok := p[key]; ok {
		if t, ok := v.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// ID returns the message ID as string from properties.
func (p Properties) ID() (string, bool) {
	return p.String(PropID)
}

// Source returns the event source URI as string from properties.
func (p Properties) Source() (string, bool) {
	return p.String(PropSource)
}

// SpecVersion returns the CloudEvents spec version as string from properties.
func (p Properties) SpecVersion() (string, bool) {
	return p.String(PropSpecVersion)
}

// Type returns the specific type name as string from properties.
func (p Properties) Type() (string, bool) {
	return p.String(PropType)
}

// Subject returns the subject as string from properties.
func (p Properties) Subject() (string, bool) {
	return p.String(PropSubject)
}

// EventTime returns the event timestamp from properties.
func (p Properties) EventTime() (time.Time, bool) {
	return p.Time(PropTime)
}

// DataContentType returns the data content type as string from properties.
func (p Properties) DataContentType() (string, bool) {
	return p.String(PropDataContentType)
}

// CorrelationID returns the correlation ID as string from properties.
func (p Properties) CorrelationID() (string, bool) {
	return p.String(PropCorrelationID)
}

// Deadline returns the deadline for processing this message.
func (p Properties) Deadline() (time.Time, bool) {
	return p.Time(PropDeadline)
}

// Topic returns the pub/sub topic as string from properties.
// Empty string is a valid topic representing the default topic.
func (p Properties) Topic() (string, bool) {
	return p.String(PropTopic)
}

// Deprecated: Use EventTime instead.
func (p Properties) CreatedAt() (time.Time, bool) {
	return p.EventTime()
}

// Deprecated: Use DataContentType instead.
func (p Properties) ContentType() (string, bool) {
	return p.DataContentType()
}
