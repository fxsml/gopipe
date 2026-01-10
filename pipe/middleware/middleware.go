// Package middleware provides composable middleware for ProcessFunc.
//
// Middleware wraps a ProcessFunc with additional behavior such as
// context management, panic recovery, retry logic, and metrics collection.
package middleware

import "context"

// ProcessFunc is the core processing signature.
// It takes a context and input, and returns zero or more outputs or an error.
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

// Middleware wraps a ProcessFunc with additional behavior.
type Middleware[In, Out any] func(ProcessFunc[In, Out]) ProcessFunc[In, Out]

