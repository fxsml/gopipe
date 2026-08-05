// Package message provides CloudEvents-aligned message handling with type-based routing.
//
// This package is part of [gopipe], a composable data pipeline toolkit for Go.
// The gopipe family includes:
//
//   - [channel] — Stateless transforms, filters, fan-in/out
//   - [pipe] — Stateful components with lifecycle management
//   - [message] (this package) — CloudEvents message routing with type-based handlers
//
// The package centers around the [Router], which dispatches messages to
// handlers by CloudEvents type. Messages follow the CloudEvents specification
// with typed data payloads and context attributes.
//
// # Quick Start
//
//	router := message.NewRouter(message.PipeConfig{})
//
//	handler := message.NewCommandHandler(
//		func(ctx context.Context, cmd OrderCmd) ([]OrderEvent, error) {
//			return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
//		},
//		message.CommandHandlerConfig{Source: "/orders", Naming: message.DotNaming},
//	)
//	router.AddHandler("orders", nil, handler)
//
//	output, _ := router.Pipe(ctx, inputCh)
//
// # Architecture
//
// [Router] is a standalone component: it takes a channel of typed [Message]
// values and returns a channel of typed [Message] values. For raw ([]byte)
// I/O — broker integration, HTTP — compose [NewUnmarshalPipe] and
// [NewMarshalPipe] at the boundary:
//
//	rawIn → NewUnmarshalPipe → Router → NewMarshalPipe → rawOut
//
// See README.md in this package for a worked example.
//
// # Design Notes
//
// Handler is self-describing via [Handler.EventType] and [Handler.NewInput],
// eliminating the need for a central type registry. [Router] and
// [UnmarshalPipe] read these methods to route messages and create instances
// for unmarshaling.
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
// # Acknowledgment
//
// Messages support acknowledgment callbacks for broker integration. All pipeline
// components auto-nack on failure (unmarshal error, no handler, handler error,
// distribution failure, shutdown). Acking on success is explicit.
//
// Basic pattern - ack after output delivery:
//
//	// Input source sets up acking
//	raw := message.NewRaw(data, attrs, message.NewAcking(
//		func() { broker.Ack(msgID) },
//		func(err error) { broker.Nack(msgID) },
//	))
//
//	// Output consumer acks after successful delivery
//	for msg := range output {
//		if err := broker.Publish(msg); err == nil {
//			msg.Ack()
//		}
//	}
//
// For automatic ack-on-handler-success, use [middleware.AutoAck]:
//
//	router.Use(middleware.AutoAck())
//
// # Batch Processing
//
// When flattening batches (1 input → N outputs), use [NewSharedAcking] for
// all-or-nothing semantics:
//
//	func flatten(batch *Message) []*Message {
//		items := extractItems(batch.Data)
//		shared := message.NewSharedAcking(
//			func() { batch.Ack() },
//			func(err error) { batch.Nack(err) },
//			len(items),
//		)
//		outputs := make([]*Message, len(items))
//		for i, item := range items {
//			outputs[i] = message.New(item, batch.Attributes, shared)
//		}
//		return outputs
//	}
//
// Alternative strategies for batches:
//   - Ack immediately, track failures via metrics/DLQ (high throughput)
//   - Route failures to dead-letter queue, ack individual items
//   - Threshold-based: nack batch only if failure rate exceeds threshold
//
// [Copy] shares the acking pointer, preserving acknowledgment through transforms.
//
// # Message Types
//
// [TypedMessage] is the generic base type. [Message] (any data) and [RawMessage]
// ([]byte data) are type aliases for common use cases.
//
// # Subpackages
//
//   - [cloudevents]: Integration with CloudEvents SDK protocol bindings
//   - [match]: Matchers for filtering messages by attributes
//   - [middleware]: Cross-cutting concerns (correlation ID, logging)
//
// [gopipe]: https://github.com/fxsml/gopipe
// [channel]: https://pkg.go.dev/github.com/fxsml/gopipe/channel
// [pipe]: https://pkg.go.dev/github.com/fxsml/gopipe/pipe
// [message]: https://pkg.go.dev/github.com/fxsml/gopipe/message
// [cloudevents]: https://pkg.go.dev/github.com/fxsml/gopipe/message/cloudevents
// [match]: https://pkg.go.dev/github.com/fxsml/gopipe/message/match
// [middleware]: https://pkg.go.dev/github.com/fxsml/gopipe/message/middleware
package message
