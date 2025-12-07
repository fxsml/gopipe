package message

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
