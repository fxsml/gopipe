// Package message provides CloudEvents-aligned message handling with type-based routing.
//
// This package is part of [gopipe], a composable data pipeline toolkit for Go.
// The gopipe family includes:
//
//   - [channel] — Stateless transforms, filters, fan-in/out
//   - [pipe] — Stateful components with lifecycle management
//   - [message] (this package) — CloudEvents message routing with type-based handlers
//
// The package centers around the [Engine], which orchestrates message flow between
// inputs, handlers, and outputs. Messages follow the CloudEvents specification
// with typed data payloads and context attributes.
//
// [gopipe]: https://github.com/fxsml/gopipe
// [channel]: https://pkg.go.dev/github.com/fxsml/gopipe/channel
// [pipe]: https://pkg.go.dev/github.com/fxsml/gopipe/pipe
// [message]: https://pkg.go.dev/github.com/fxsml/gopipe/message
//
// # Quick Start
//
//	engine := message.NewEngine(message.EngineConfig{
//		Marshaler: message.NewJSONMarshaler(),
//	})
//
//	handler := message.NewCommandHandler(
//		func(ctx context.Context, cmd OrderCmd) ([]OrderEvent, error) {
//			return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
//		},
//		message.CommandHandlerConfig{Source: "/orders", Naming: message.KebabNaming},
//	)
//	engine.AddHandler("orders", nil, handler)
//
//	engine.AddRawInput("in", nil, inputCh)
//	output, _ := engine.AddRawOutput("out", nil)
//
//	done, _ := engine.Start(ctx)
//
// # Architecture
//
// The engine uses a single merger for all inputs. Each raw input has its own
// unmarshal pipe that feeds typed messages into the shared merger. Typed inputs
// feed directly into the merger, then route to handlers via the router.
//
// Loopback is not built into Engine—use [plugin.Loopback] which connects a
// TypedOutput back to TypedInput via the existing Add* APIs.
//
// See README.md in this package for detailed architecture diagrams.
//
// # Design Notes
//
// Handler is self-describing via [Handler.EventType] and [Handler.NewInput],
// eliminating the need for a central type registry. The engine reads these
// methods to route messages and create instances for unmarshaling.
//
// [Matcher.Match] uses [Attributes] instead of *Message because all matchers
// only access attributes, avoiding allocation when matching raw messages.
//
// Add* methods take direct parameters (name, matcher) rather than config
// structs for simplicity. Config structs are reserved for constructors
// where extensibility is more important.
//
// For rejected alternatives and common mistakes, see AGENTS.md in the
// repository root.
//
// # Message Types
//
// [TypedMessage] is the generic base type. [Message] (any data) and [RawMessage]
// ([]byte data) are type aliases for common use cases.
//
// # Subpackages
//
// match: Matchers for filtering messages by attributes
//
// middleware: Cross-cutting concerns (correlation ID, logging)
//
// plugin: Reusable engine plugins (Loopback, ProcessLoopback)
package message
