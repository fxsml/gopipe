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

	// PropSubject indicates the subject of the message.
	PropSubject = "subject"

	// PropContentType indicates the content type of the message.
	PropContentType = "content_type"
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
