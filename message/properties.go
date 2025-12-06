package message

// Reserved property keys for gopipe internal use.
// User-defined properties should NOT use the "gopipe." prefix.
const (
	// PropID is the unique message identifier.
	PropID = "gopipe.message.id"

	// PropCorrelationID is used to correlate related messages across services.
	PropCorrelationID = "gopipe.message.correlation_id"

	// PropCreatedAt stores when the message was created.
	PropCreatedAt = "gopipe.message.created_at"

	// PropDeadline stores the message processing deadline.
	PropDeadline = "gopipe.message.deadline"

	// PropSubject indicates the subject of the message.
	PropSubject = "gopipe.message.subject"

	// PropContentType indicates the content type of the message.
	PropContentType = "gopipe.message.content_type"
)
