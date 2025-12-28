package match

import "github.com/fxsml/gopipe/message"

// allMatcher combines matchers with AND logic.
type allMatcher struct {
	matchers []message.Matcher
}

// All creates a matcher that requires all matchers to match (AND).
func All(matchers ...message.Matcher) message.Matcher {
	return &allMatcher{matchers: matchers}
}

func (m *allMatcher) Match(attrs message.Attributes) bool {
	for _, matcher := range m.matchers {
		if !matcher.Match(attrs) {
			return false
		}
	}
	return true
}

// anyMatcher combines matchers with OR logic.
type anyMatcher struct {
	matchers []message.Matcher
}

// Any creates a matcher that requires any matcher to match (OR).
func Any(matchers ...message.Matcher) message.Matcher {
	return &anyMatcher{matchers: matchers}
}

func (m *anyMatcher) Match(attrs message.Attributes) bool {
	for _, matcher := range m.matchers {
		if matcher.Match(attrs) {
			return true
		}
	}
	return false
}
