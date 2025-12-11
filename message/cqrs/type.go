package cqrs

import "reflect"

// TypeOf returns the reflected type name of the given value.
func TypeOf(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return "nil"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// GenericTypeOf returns the reflected type name of the generic type T.
func GenericTypeOf[T any]() string {
	var zero T
	return TypeOf(zero)
}
