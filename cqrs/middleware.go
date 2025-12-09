package cqrs

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// CorrelationMiddleware returns middleware that propagates correlation ID from input to output messages.
//
// This middleware ensures that correlation IDs are automatically carried forward from input messages
// to all output messages, enabling request tracing across message processing pipelines.
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            cqrs.CorrelationMiddleware(),
//	        },
//	    },
//	    handlers...,
//	)
func CorrelationMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				results, err := proc.Process(ctx, msg)
				if err != nil {
					return results, err
				}

				// Propagate correlation ID to all output messages
				if corrID, ok := msg.Properties.CorrelationID(); ok {
					for _, outMsg := range results {
						if outMsg.Properties == nil {
							outMsg.Properties = make(message.Properties)
						}
						// Always overwrite correlation ID
						outMsg.Properties[message.PropCorrelationID] = corrID
					}
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}

// TypeMiddleware returns middleware that sets the type property on all output messages.
//
// This middleware is useful for ensuring all output messages have a consistent type,
// such as "event", "command", or "query".
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            cqrs.TypeMiddleware("event"),
//	        },
//	    },
//	    handlers...,
//	)
func TypeMiddleware(msgType string) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				results, err := proc.Process(ctx, msg)
				if err != nil {
					return results, err
				}

				for _, outMsg := range results {
					if outMsg.Properties == nil {
						outMsg.Properties = make(message.Properties)
					}
					outMsg.Properties["type"] = msgType
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}

// SubjectMiddleware returns middleware that sets the subject property on all output messages.
//
// This middleware is useful for routing messages or categorizing them by topic.
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            cqrs.SubjectMiddleware("OrderEvents"),
//	        },
//	    },
//	    handlers...,
//	)
func SubjectMiddleware(subject string) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				results, err := proc.Process(ctx, msg)
				if err != nil {
					return results, err
				}

				for _, outMsg := range results {
					if outMsg.Properties == nil {
						outMsg.Properties = make(message.Properties)
					}
					outMsg.Properties[message.PropSubject] = subject
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}

// SubjectAndTypeMiddleware returns middleware that sets both subject and type on all output messages.
//
// This is more efficient than applying SubjectMiddleware and TypeMiddleware separately.
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            cqrs.SubjectAndTypeMiddleware("OrderCreated", "event"),
//	        },
//	    },
//	    handlers...,
//	)
func SubjectAndTypeMiddleware(subject, msgType string) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				results, err := proc.Process(ctx, msg)
				if err != nil {
					return results, err
				}

				for _, outMsg := range results {
					if outMsg.Properties == nil {
						outMsg.Properties = make(message.Properties)
					}
					outMsg.Properties[message.PropSubject] = subject
					outMsg.Properties["type"] = msgType
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}

// TypeAndNameMiddleware returns middleware that sets type and derives subject from the payload type name.
//
// This middleware uses reflection to extract the type name from the generic parameter T
// and uses it as the subject for all output messages. This is useful for automatic routing
// based on message types.
//
// Example:
//
//	type OrderCreated struct { ... }
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            cqrs.TypeAndNameMiddleware[OrderCreated]("event"),
//	        },
//	    },
//	    handlers...,
//	)
//
// Output messages will have:
// - subject: "OrderCreated"
// - type: "event"
func TypeAndNameMiddleware[T any](msgType string) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	typeName := typeNameOf[T]()
	return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
		return gopipe.NewProcessor(
			func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
				results, err := proc.Process(ctx, msg)
				if err != nil {
					return results, err
				}

				for _, outMsg := range results {
					if outMsg.Properties == nil {
						outMsg.Properties = make(message.Properties)
					}
					outMsg.Properties[message.PropSubject] = typeName
					outMsg.Properties["type"] = msgType
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}
