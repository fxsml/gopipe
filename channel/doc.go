// Package channel provides stateless channel operations for composing data pipelines.
//
// Operations include filtering, transforming, merging, routing, broadcasting,
// batching, and more. All functions are pure and create new channels without
// modifying inputs.
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
