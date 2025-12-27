package match

import "github.com/fxsml/gopipe/message"

// typesMatcher matches messages by CE type patterns.
type typesMatcher struct {
	patterns []string
}

// Types creates a matcher that matches CE type against patterns.
// Uses SQL LIKE syntax: % = any sequence, _ = single char.
func Types(patterns ...string) message.Matcher {
	return &typesMatcher{patterns: patterns}
}

func (m *typesMatcher) Match(msg *message.Message) bool {
	if msg == nil {
		return false
	}
	ceType := getAttr(msg.Attributes, "type")
	return LikeAny(m.patterns, ceType)
}
