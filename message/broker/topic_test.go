package broker_test

import (
	"testing"

	"github.com/fxsml/gopipe/message/broker"
)

func TestTopicMatcher_ExactMatch(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		{"orders", "orders", true},
		{"orders", "users", false},
		{"orders/created", "orders/created", true},
		{"orders/created", "orders/updated", false},
		{"a/b/c", "a/b/c", true},
		{"a/b/c", "a/b/d", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			m := broker.NewTopicMatcher(tt.pattern)
			if got := m.Matches(tt.topic); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestTopicMatcher_SingleLevelWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		{"orders/+", "orders/created", true},
		{"orders/+", "orders/updated", true},
		{"orders/+", "orders", false},
		{"orders/+", "orders/created/v2", false},
		{"+/created", "orders/created", true},
		{"+/created", "users/created", true},
		{"+/+", "a/b", true},
		{"+/+/+", "a/b/c", true},
		{"a/+/c", "a/b/c", true},
		{"a/+/c", "a/x/c", true},
		{"a/+/c", "a/b/d", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			m := broker.NewTopicMatcher(tt.pattern)
			if got := m.Matches(tt.topic); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestTopicMatcher_MultiLevelWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		{"orders/#", "orders", true},
		{"orders/#", "orders/created", true},
		{"orders/#", "orders/created/v2", true},
		{"orders/#", "users/created", false},
		{"#", "anything", true},
		{"#", "a/b/c/d", true},
		{"a/b/#", "a/b", true},
		{"a/b/#", "a/b/c", true},
		{"a/b/#", "a/b/c/d/e", true},
		{"a/b/#", "a/c", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			m := broker.NewTopicMatcher(tt.pattern)
			if got := m.Matches(tt.topic); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestTopicMatcher_CombinedWildcards(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		{"+/orders/#", "us/orders", true},
		{"+/orders/#", "us/orders/created", true},
		{"+/orders/#", "eu/orders/created/v2", true},
		{"+/+/#", "a/b/c/d/e", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			m := broker.NewTopicMatcher(tt.pattern)
			if got := m.Matches(tt.topic); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestSplitTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  []string
	}{
		{"", nil},
		{"orders", []string{"orders"}},
		{"orders/created", []string{"orders", "created"}},
		{"a/b/c/d", []string{"a", "b", "c", "d"}},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := broker.SplitTopic(tt.topic)
			if len(got) != len(tt.want) {
				t.Errorf("SplitTopic(%q) = %v, want %v", tt.topic, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SplitTopic(%q)[%d] = %q, want %q", tt.topic, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestJoinTopic(t *testing.T) {
	tests := []struct {
		segments []string
		want     string
	}{
		{nil, ""},
		{[]string{"orders"}, "orders"},
		{[]string{"orders", "created"}, "orders/created"},
		{[]string{"a", "b", "c"}, "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := broker.JoinTopic(tt.segments...)
			if got != tt.want {
				t.Errorf("JoinTopic(%v) = %q, want %q", tt.segments, got, tt.want)
			}
		})
	}
}

func TestParentTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"", ""},
		{"orders", ""},
		{"orders/created", "orders"},
		{"a/b/c", "a/b"},
		{"a/b/c/d", "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := broker.ParentTopic(tt.topic)
			if got != tt.want {
				t.Errorf("ParentTopic(%q) = %q, want %q", tt.topic, got, tt.want)
			}
		})
	}
}

func TestBaseTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"", ""},
		{"orders", "orders"},
		{"orders/created", "created"},
		{"a/b/c", "c"},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := broker.BaseTopic(tt.topic)
			if got != tt.want {
				t.Errorf("BaseTopic(%q) = %q, want %q", tt.topic, got, tt.want)
			}
		})
	}
}
