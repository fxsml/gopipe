package middleware

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// MessageSubject returns middleware that sets the subject property on all output messages.
//
// This middleware is useful for routing messages or categorizing them by topic.
//
// Example:
//
//	router := message.NewRouter(
//	    message.RouterConfig{
//	        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
//	            middleware.MessageSubject("OrderEvents"),
//	        },
//	    },
//	    handlers...,
//	)
func MessageSubject(subject string) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
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
