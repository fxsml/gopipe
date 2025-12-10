package message

import "time"

// Reserved property keys for standard message metadata.
const (
	// PropID is the unique message identifier.
	PropID = "message_id"

	// PropCorrelationID is used to correlate related messages across services.
	PropCorrelationID = "correlation_id"

	// PropCreatedAt stores when the message was created.
	PropCreatedAt = "created_at"

	// PropDeadline stores the message processing deadline.
	PropDeadline = "deadline"

	// PropSubject identifies the specific entity this message is about within the source.
	// Per CloudEvents spec: "the subject of the event in the context of the event producer"
	// Example: "order/ORD-001" for an order-related event, "user/123" for a user event.
	PropSubject = "subject"

	// PropContentType indicates the content type of the message.
	PropContentType = "content_type"

	// PropType describes what kind of event/command occurred.
	// Per CloudEvents spec: "contains a value describing the type of event related to the originating occurrence"
	// Examples: "OrderCreated", "CreateOrder", "com.example.order.created"
	// Used for routing and handler matching. Should NOT be generic values like "event" or "command".
	PropType = "type"

	// PropTopic is the pub/sub topic for routing messages in publish-subscribe systems.
	// This property is used by publishers to determine message routing and should be set
	// by PropertyProviders or marshalers. Empty string is a valid topic value representing
	// the default topic. Senders should not forward this property to the underlying broker.
	PropTopic = "topic"
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

// CorrelationID returns the correlation ID as string from properties.
func (p Properties) CorrelationID() (string, bool) {
	return p.String(PropCorrelationID)
}

// CreatedAt returns the created timestamp from properties.
func (p Properties) CreatedAt() (time.Time, bool) {
	return p.Time(PropCreatedAt)
}

// Subject returns the subject as string from properties.
func (p Properties) Subject() (string, bool) {
	return p.String(PropSubject)
}

// ContentType returns the content type as string from properties.
func (p Properties) ContentType() (string, bool) {
	return p.String(PropContentType)
}

// Deadline returns the deadline for processing this message.
func (p Properties) Deadline() (time.Time, bool) {
	return p.Time(PropDeadline)
}

// Type returns the specific type name as string from properties.
func (p Properties) Type() (string, bool) {
	return p.String(PropType)
}

// Topic returns the pub/sub topic as string from properties.
// Empty string is a valid topic representing the default topic.
func (p Properties) Topic() (string, bool) {
	return p.String(PropTopic)
}
