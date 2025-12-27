package message

import (
	"reflect"
	"strings"
	"unicode"
)

// NamingStrategy derives CloudEvents type names from Go types.
type NamingStrategy interface {
	TypeName(t reflect.Type) string
}

// KebabNaming converts PascalCase to dot-separated lowercase.
// Example: OrderCreated → "order.created"
var KebabNaming NamingStrategy = kebabNaming{}

// SnakeNaming converts PascalCase to underscore-separated lowercase.
// Example: OrderCreated → "order_created"
var SnakeNaming NamingStrategy = snakeNaming{}

type kebabNaming struct{}

func (kebabNaming) TypeName(t reflect.Type) string {
	return splitPascalCase(t.Name(), ".")
}

type snakeNaming struct{}

func (snakeNaming) TypeName(t reflect.Type) string {
	return splitPascalCase(t.Name(), "_")
}

// splitPascalCase splits a PascalCase string into lowercase words joined by sep.
func splitPascalCase(s string, sep string) string {
	if s == "" {
		return ""
	}

	var words []string
	var current strings.Builder

	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			words = append(words, strings.ToLower(current.String()))
			current.Reset()
		}
		current.WriteRune(r)
	}

	if current.Len() > 0 {
		words = append(words, strings.ToLower(current.String()))
	}

	return strings.Join(words, sep)
}
