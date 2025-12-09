package middleware

import (
	"context"
	"reflect"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// MessageTypeName returns middleware that sets the subject property based on the reflected type name of T.
//
// This middleware uses reflection to extract the type name from the generic parameter T
// and uses it as the subject for all output messages. This is useful for automatic routing
// based on message types.
//
// Combine with MessageType() to set both subject and type:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            middleware.MessageTypeName[OrderCreated](),
//	            middleware.MessageType("event"),
//	        },
//	    },
//	    handlers...,
//	)
//
// Output messages will have:
// - subject: "OrderCreated" (from type name)
// - type: "event" (from MessageType middleware)
func MessageTypeName[T any]() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
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
				}

				return results, err
			},
			func(msg *message.Message, err error) {
				proc.Cancel(msg, err)
			},
		)
	}
}

// typeNameOf returns the type name of T, stripping pointer indirection.
func typeNameOf[T any]() string {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
