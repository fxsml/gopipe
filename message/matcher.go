package message

// Matcher tests whether a message matches a condition.
// Used for input filtering and output routing.
type Matcher interface {
	Match(msg *Message) bool
}
