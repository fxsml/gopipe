// Package channel provides stateless channel operations for composing data pipelines.
//
// This package is part of [gopipe], a composable data pipeline toolkit for Go.
// The gopipe family includes:
//
//   - [channel] (this package) — Stateless transforms, filters, fan-in/out
//   - [pipe] — Stateful components with lifecycle management
//   - [message] — CloudEvents message routing with type-based handlers
//
// Operations include filtering, transforming, merging, routing, broadcasting,
// batching, and more. All functions are pure and create new channels without
// modifying inputs.
//
// [gopipe]: https://github.com/fxsml/gopipe
// [channel]: https://pkg.go.dev/github.com/fxsml/gopipe/channel
// [pipe]: https://pkg.go.dev/github.com/fxsml/gopipe/pipe
// [message]: https://pkg.go.dev/github.com/fxsml/gopipe/message
//
// # Quick Start
//
//	// Generate, filter, transform, consume
//	in := channel.FromRange(10)
//	filtered := channel.Filter(in, func(i int) bool { return i%2 == 0 })
//	transformed := channel.Transform(filtered, func(i int) string { return fmt.Sprint(i) })
//	<-channel.Sink(transformed, func(s string) { fmt.Println(s) })
//
// # Categories
//
// Sources: [FromSlice], [FromRange], [FromValues], [FromFunc]
//
// Transforms: [Filter], [Transform], [Process], [Flatten], [Buffer]
//
// Fan-out: [Broadcast], [Route]
//
// Fan-in: [Merge]
//
// Sinks: [Sink], [Drain], [ToSlice]
//
// Batching: [Batch], [Collect], [GroupBy]
//
// For stateful components with lifecycle management, see the pipe package.
package channel
