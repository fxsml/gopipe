package match

import "github.com/fxsml/gopipe/message"

// sourcesMatcher matches messages by CE source patterns.
type sourcesMatcher struct {
	patterns []string
}

// Sources creates a matcher that matches CE source against patterns.
// Uses SQL LIKE syntax: % = any sequence, _ = single char.
func Sources(patterns ...string) message.Matcher {
	return &sourcesMatcher{patterns: patterns}
}

func (m *sourcesMatcher) Match(attrs message.Attributes) bool {
	source := getAttr(attrs, "source")
	return LikeAny(m.patterns, source)
}
