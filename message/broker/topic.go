package broker

import (
	"strings"
)

// TopicMatcher provides pattern matching for hierarchical topics.
// Topics are "/" separated strings like "orders/created" or "users/profile/updated".
type TopicMatcher struct {
	pattern  string
	segments []string
}

// NewTopicMatcher creates a new topic matcher for the given pattern.
// Patterns support:
//   - Exact match: "orders/created" matches only "orders/created"
//   - Single-level wildcard (+): "orders/+" matches "orders/created", "orders/updated"
//   - Multi-level wildcard (#): "orders/#" matches "orders/created", "orders/created/v2"
func NewTopicMatcher(pattern string) *TopicMatcher {
	return &TopicMatcher{
		pattern:  pattern,
		segments: strings.Split(pattern, "/"),
	}
}

// Matches returns true if the topic matches the pattern.
func (tm *TopicMatcher) Matches(topic string) bool {
	topicSegments := strings.Split(topic, "/")
	return matchSegments(tm.segments, topicSegments)
}

func matchSegments(pattern, topic []string) bool {
	pi, ti := 0, 0

	for pi < len(pattern) && ti < len(topic) {
		switch pattern[pi] {
		case "#":
			// Multi-level wildcard matches everything remaining
			return true
		case "+":
			// Single-level wildcard matches exactly one segment
			pi++
			ti++
		default:
			// Exact match required
			if pattern[pi] != topic[ti] {
				return false
			}
			pi++
			ti++
		}
	}

	// Check if both pattern and topic are exhausted
	if pi == len(pattern) && ti == len(topic) {
		return true
	}

	// Pattern ends with # and we consumed it
	if pi < len(pattern) && pattern[pi] == "#" {
		return true
	}

	return false
}

// SplitTopic splits a topic string into its segments.
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
