package message

// Matcher tests whether attributes match a condition.
// Used for input filtering and output routing.
// Operates on Attributes only (not full Message) to avoid wrapper allocation.
type Matcher interface {
	Match(attrs Attributes) bool
}
