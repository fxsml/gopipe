package match

import (
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestAll(t *testing.T) {
	t.Run("matches when all matchers match", func(t *testing.T) {
		m := All(
			Types("order.%"),
			Sources("/api/%"),
		)
		attrs := message.Attributes{
			"type":   "order.created",
			"source": "/api/orders",
		}
		if !m.Match(attrs) {
			t.Error("All() should match when all matchers match")
		}
	})

	t.Run("fails when any matcher fails", func(t *testing.T) {
		m := All(
			Types("order.%"),
			Sources("/api/%"),
		)
		attrs := message.Attributes{
			"type":   "order.created",
			"source": "/web/orders",
		}
		if m.Match(attrs) {
			t.Error("All() should fail when any matcher fails")
		}
	})

	t.Run("matches with no matchers", func(t *testing.T) {
		m := All()
		if !m.Match(message.Attributes{}) {
			t.Error("All() with no matchers should match")
		}
	})
}

func TestAny(t *testing.T) {
	t.Run("matches when any matcher matches", func(t *testing.T) {
		m := Any(
			Types("order.%"),
			Types("user.%"),
		)
		attrs := message.Attributes{"type": "user.created"}
		if !m.Match(attrs) {
			t.Error("Any() should match when any matcher matches")
		}
	})

	t.Run("fails when no matcher matches", func(t *testing.T) {
		m := Any(
			Types("order.%"),
			Types("user.%"),
		)
		attrs := message.Attributes{"type": "product.created"}
		if m.Match(attrs) {
			t.Error("Any() should fail when no matcher matches")
		}
	})

	t.Run("fails with no matchers", func(t *testing.T) {
		m := Any()
		if m.Match(message.Attributes{}) {
			t.Error("Any() with no matchers should not match")
		}
	})
}

func TestTypes(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		eventType   string
		want     bool
	}{
		{"exact match", []string{"order.created"}, "order.created", true},
		{"no match", []string{"order.created"}, "order.updated", false},
		{"wildcard suffix", []string{"order.%"}, "order.created", true},
		{"wildcard prefix", []string{"%.created"}, "order.created", true},
		{"wildcard middle", []string{"order.%.v1"}, "order.created.v1", true},
		{"single char", []string{"order.create_"}, "order.created", true},
		{"multiple patterns", []string{"order.%", "user.%"}, "user.created", true},
		{"empty type", []string{"order.%"}, "", false},
		{"empty patterns", []string{}, "order.created", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Types(tt.patterns...)
			attrs := message.Attributes{"type": tt.eventType}
			if got := m.Match(attrs); got != tt.want {
				t.Errorf("Types(%v).Match(%q) = %v, want %v", tt.patterns, tt.eventType, got, tt.want)
			}
		})
	}
}

func TestSources(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		source   string
		want     bool
	}{
		{"exact match", []string{"/api/orders"}, "/api/orders", true},
		{"no match", []string{"/api/orders"}, "/api/users", false},
		{"wildcard suffix", []string{"/api/%"}, "/api/orders", true},
		{"wildcard prefix", []string{"%/orders"}, "/api/orders", true},
		{"multiple patterns", []string{"/api/%", "/web/%"}, "/web/orders", true},
		{"empty source", []string{"/api/%"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Sources(tt.patterns...)
			attrs := message.Attributes{"source": tt.source}
			if got := m.Match(attrs); got != tt.want {
				t.Errorf("Sources(%v).Match(%q) = %v, want %v", tt.patterns, tt.source, got, tt.want)
			}
		})
	}
}

func TestLike(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		// Exact matches
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"", "", true},

		// Percent wildcard
		{"%", "", true},
		{"%", "anything", true},
		{"a%", "abc", true},
		{"a%", "a", true},
		{"a%", "bcd", false},
		{"%c", "abc", true},
		{"%c", "c", true},
		{"%c", "cd", false},
		{"a%c", "ac", true},
		{"a%c", "abc", true},
		{"a%c", "abbc", true},
		{"a%c", "ab", false},
		{"%b%", "abc", true},
		{"%b%", "b", true},
		{"%b%", "ac", false},

		// Underscore wildcard
		{"_", "a", true},
		{"_", "", false},
		{"_", "ab", false},
		{"a_c", "abc", true},
		{"a_c", "ac", false},
		{"a_c", "abbc", false},
		{"___", "abc", true},
		{"___", "ab", false},

		// Combined wildcards
		{"a_%", "ab", true},
		{"a_%", "abc", true},
		{"a_%", "a", false},
		{"%_c", "abc", true},
		{"%_c", "c", false},
		{"a%_c", "abc", true},
		{"a%_c", "abxc", true},
		{"a%_c", "ac", false},

		// Real-world patterns
		{"order.%", "order.created", true},
		{"order.%", "order.", true},
		{"%.created", "order.created", true},
		{"order._.v1", "order.a.v1", true},
		{"order._.v1", "order.ab.v1", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			if got := Like(tt.pattern, tt.value); got != tt.want {
				t.Errorf("Like(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

func TestCombinedMatchers(t *testing.T) {
	t.Run("nested All and Any", func(t *testing.T) {
		// Match orders from API or users from web
		m := Any(
			All(Types("order.%"), Sources("/api/%")),
			All(Types("user.%"), Sources("/web/%")),
		)

		cases := []struct {
			attrs message.Attributes
			want  bool
		}{
			{message.Attributes{"type": "order.created", "source": "/api/orders"}, true},
			{message.Attributes{"type": "user.created", "source": "/web/users"}, true},
			{message.Attributes{"type": "order.created", "source": "/web/orders"}, false},
			{message.Attributes{"type": "user.created", "source": "/api/users"}, false},
		}

		for _, c := range cases {
			if got := m.Match(c.attrs); got != c.want {
				t.Errorf("Match(%v) = %v, want %v", c.attrs, got, c.want)
			}
		}
	})
}
