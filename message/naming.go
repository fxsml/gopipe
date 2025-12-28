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
// Consecutive uppercase letters are treated as acronyms:
//   - HTTPRequest → http.request
//   - IOBroker → io.broker
//   - ID → id
//   - OrderCreated → order.created
func splitPascalCase(s string, sep string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	var words []string
	var current strings.Builder

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if i > 0 && unicode.IsUpper(r) {
			// Check if this is the start of a new word or part of an acronym
			prevUpper := unicode.IsUpper(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])

			// Start new word if:
			// 1. Previous was lowercase (normal word boundary: orderC → order, C)
			// 2. Previous was uppercase AND next is lowercase (acronym end: HTTPr → HTTP, r)
			if !prevUpper || (prevUpper && nextLower) {
				words = append(words, strings.ToLower(current.String()))
				current.Reset()
			}
		}
		current.WriteRune(r)
	}

	if current.Len() > 0 {
		words = append(words, strings.ToLower(current.String()))
	}

	return strings.Join(words, sep)
}
