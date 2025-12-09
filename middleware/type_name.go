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
	return NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			results, err := next()
			if err != nil {
				return results, err
			}

			for _, outMsg := range results {
				if outMsg.Properties == nil {
					outMsg.Properties = make(message.Properties)
				}
				outMsg.Properties[message.PropSubject] = typeName
			}

			return results, nil
		},
	)
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
