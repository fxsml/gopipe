package message

// Matcher tests whether attributes match a condition.
// Used for input filtering and output routing.
// Operates on Attributes only (not full Message) to work with both
// Message and RawMessage without wrapper allocation.
type Matcher interface {
	Match(attrs Attributes) bool
}
