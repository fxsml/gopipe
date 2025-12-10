package middleware

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// MessageType returns middleware that sets the type attribute on output messages
// based on the concrete type of each message's data payload.
func MessageType() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
	return NewMessageMiddleware(
		func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
			results, err := next()
			if err != nil {
				return results, err
			}

			for _, outMsg := range results {
				// Determine type from message data
				typeName := determineType(outMsg.Data)
				if typeName == "" {
					continue
				}

				if outMsg.Attributes == nil {
					outMsg.Attributes = make(message.Attributes)
				}
				outMsg.Attributes[message.AttrType] = typeName
			}

			return results, nil
		},
	)
}

// determineType extracts the type name from the JSON data payload.
func determineType(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err == nil {
		t := reflect.TypeOf(v)
		if t == nil {
			return ""
		}
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		// Return named types if available
		if name := t.Name(); name != "" {
			return name
		}

		// For unnamed types, return the kind
		switch t.Kind() {
		case reflect.Map:
			return "map"
		case reflect.Slice:
			return "slice"
		case reflect.String:
			return "string"
		case reflect.Bool:
			return "bool"
		case reflect.Float64, reflect.Float32:
			return "number"
		default:
			return t.Kind().String()
		}
	}

	return ""
}
