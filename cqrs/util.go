package cqrs

import (
	"reflect"

	"github.com/fxsml/gopipe/message"
)

// CreateCommand creates a command message from a command struct.
func CreateCommand(marshaler Marshaler, cmd any, attrs message.Attributes) *message.Message {
	payload, _ := marshaler.Marshal(cmd)

	if attrs == nil {
		attrs = message.Attributes{}
	}

	// Extract type name from the command
	t := reflect.TypeOf(cmd)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	attrs[message.AttrSubject] = t.Name()
	attrs[message.AttrType] = t.Name()
	attrs["type"] = "command"

	return message.New(payload, attrs)
}

// CreateCommands creates multiple command messages with optional correlation ID.
func CreateCommands(marshaler Marshaler, correlationID string, cmds ...any) []*message.Message {
	msgs := make([]*message.Message, 0, len(cmds))

	for _, cmd := range cmds {
		attrs := message.Attributes{}
		if correlationID != "" {
			attrs[message.AttrCorrelationID] = correlationID
		}

		msgs = append(msgs, CreateCommand(marshaler, cmd, attrs))
	}

	return msgs
}
