package message

import (
	"reflect"
	"testing"
)

type OrderCreated struct{}
type UserSignedUp struct{}
type HTTPRequest struct{}
type ID struct{}

func TestKebabNaming(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{OrderCreated{}, "order.created"},
		{UserSignedUp{}, "user.signed.up"},
		{HTTPRequest{}, "h.t.t.p.request"},
		{ID{}, "i.d"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := KebabNaming.TypeName(reflect.TypeOf(tt.input))
			if result != tt.expected {
				t.Errorf("KebabNaming(%T) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSnakeNaming(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{OrderCreated{}, "order_created"},
		{UserSignedUp{}, "user_signed_up"},
		{HTTPRequest{}, "h_t_t_p_request"},
		{ID{}, "i_d"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := SnakeNaming.TypeName(reflect.TypeOf(tt.input))
			if result != tt.expected {
				t.Errorf("SnakeNaming(%T) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSplitPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		sep      string
		expected string
	}{
		{"", ".", ""},
		{"A", ".", "a"},
		{"AB", ".", "a.b"},
		{"OrderCreated", ".", "order.created"},
		{"OrderCreated", "_", "order_created"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitPascalCase(tt.input, tt.sep)
			if result != tt.expected {
				t.Errorf("splitPascalCase(%q, %q) = %q, want %q", tt.input, tt.sep, result, tt.expected)
			}
		})
	}
}
