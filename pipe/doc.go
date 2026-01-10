// Package pipe provides stateful pipeline components with lifecycle management.
//
// Unlike the channel package which provides stateless operations, pipe components
// have configuration, middleware support, and controlled shutdown behavior.
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
