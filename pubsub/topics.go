package pubsub

import (
	"strings"
)

// SplitTopic splits a topic string into its segments.
// Topics use "/" as separator (e.g., "orders/created").
func SplitTopic(topic string) []string {
	if topic == "" {
		return nil
	}
	return strings.Split(topic, "/")
}

// JoinTopic joins topic segments into a topic string.
func JoinTopic(segments ...string) string {
	return strings.Join(segments, "/")
}

// ParentTopic returns the parent topic of the given topic.
// Returns empty string if the topic has no parent.
func ParentTopic(topic string) string {
	segments := SplitTopic(topic)
	if len(segments) <= 1 {
		return ""
	}
	return JoinTopic(segments[:len(segments)-1]...)
}

// BaseTopic returns the last segment of the topic.
func BaseTopic(topic string) string {
	segments := SplitTopic(topic)
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}
