// Package pipe provides stateful pipeline components with lifecycle management.
//
// This package is part of [gopipe], a composable data pipeline toolkit for Go.
// The gopipe family includes:
//
//   - [channel] — Stateless transforms, filters, fan-in/out
//   - [pipe] (this package) — Stateful components with lifecycle management
//   - [message] — CloudEvents message routing with type-based handlers
//
// Unlike the channel package which provides stateless operations, pipe components
// have configuration, middleware support, and controlled shutdown behavior.
//
// [gopipe]: https://github.com/fxsml/gopipe
// [channel]: https://pkg.go.dev/github.com/fxsml/gopipe/channel
// [pipe]: https://pkg.go.dev/github.com/fxsml/gopipe/pipe
// [message]: https://pkg.go.dev/github.com/fxsml/gopipe/message
//
// # Quick Start
//
//	p := pipe.NewProcessPipe(
//		func(ctx context.Context, in string) ([]int, error) {
//			n, err := strconv.Atoi(in)
//			return []int{n}, err
//		},
//		pipe.Config{Concurrency: 4, BufferSize: 10},
//	)
//	out, _ := p.Pipe(ctx, input)
//
// # Components
//
// Pipes: [NewProcessPipe], [NewTransformPipe], [NewBatchPipe]
//
// Dynamic fan-in: [Merger] - add inputs at runtime
//
// Dynamic fan-out: [Distributor] - add outputs with matchers at runtime
//
// Generation: [Generator] - produce values on demand
//
// # Middleware
//
// Pipes support middleware for cross-cutting concerns:
//
//	p.Use(middleware.Recover[In, Out]())
//	p.Use(middleware.Retry[In, Out](middleware.RetryConfig{MaxAttempts: 3}))
//
// For stateless channel operations, see the channel package.
// For CloudEvents message handling, see the message package.
package pipe
