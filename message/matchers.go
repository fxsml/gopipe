package message

import "reflect"

// MatchSubjectAndType returns a matcher function that matches messages
// with the specified subject and type properties.
//
// Example:
//
//	matcher := message.MatchSubjectAndType("CreateOrder", "command")
//	// Matches messages with subject="CreateOrder" and type="command"
func MatchSubjectAndType(subject, msgType string) func(Properties) bool {
	return func(prop Properties) bool {
		propSubject, _ := prop.Subject()
		propType, _ := prop["type"].(string)
		return propSubject == subject && propType == msgType
	}
}

// MatchSubject returns a matcher function that matches messages
// with the specified subject property.
//
// Example:
//
//	matcher := message.MatchSubject("CreateOrder")
//	// Matches messages with subject="CreateOrder"
func MatchSubject(subject string) func(Properties) bool {
	return func(prop Properties) bool {
		propSubject, _ := prop.Subject()
		return propSubject == subject
	}
}

// MatchType returns a matcher function that matches messages
// with the specified type property.
//
// Example:
//
//	matcher := message.MatchType("command")
//	// Matches messages with type="command"
func MatchType(msgType string) func(Properties) bool {
	return func(prop Properties) bool {
		propType, _ := prop["type"].(string)
		return propType == msgType
	}
}

// MatchTypeName returns a matcher function that matches messages
// where the "type" property equals the reflected type name of T.
//
// This is useful for automatic type-based routing.
//
// Example:
//
//	type CreateOrder struct { ... }
//
//	matcher := message.MatchTypeName[CreateOrder]()
//	// Matches messages with type="CreateOrder"
func MatchTypeName[T any]() func(Properties) bool {
	typeName := typeNameOf[T]()
	return func(prop Properties) bool {
		propType, _ := prop["type"].(string)
		return propType == typeName
	}
}

// MatchSubjectAndTypeName returns a matcher function that matches messages
// with the specified subject and where the "type" property equals the
// reflected type name of T.
//
// Example:
//
//	type CreateOrder struct { ... }
//
//	matcher := message.MatchSubjectAndTypeName[CreateOrder]("commands")
//	// Matches messages with subject="commands" and type="CreateOrder"
func MatchSubjectAndTypeName[T any](subject string) func(Properties) bool {
	typeName := typeNameOf[T]()
	return func(prop Properties) bool {
		propSubject, _ := prop.Subject()
		propType, _ := prop["type"].(string)
		return propSubject == subject && propType == typeName
	}
}

// PropagateCorrelationWithType returns a property transformation function
// that propagates the correlation ID and sets the message type.
//
// Example:
//
//	props := message.PropagateCorrelationWithType("event")
//	// Output properties will have correlation ID (if present) and type="event"
func PropagateCorrelationWithType(msgType string) func(Properties) Properties {
	return func(inProp Properties) Properties {
		props := Properties{
			"type": msgType,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[PropCorrelationID] = corrID
		}
		return props
	}
}

// PropagateCorrelationWithSubjectAndType returns a property transformation function
// that propagates the correlation ID and sets the subject and type.
//
// For command handlers that need to set both subject and type on output events.
//
// Example:
//
//	props := message.PropagateCorrelationWithSubjectAndType("OrderCreated", "event")
//	// Output properties will have subject="OrderCreated", type="event", and correlation ID
func PropagateCorrelationWithSubjectAndType(subject, msgType string) func(Properties) Properties {
	return func(inProp Properties) Properties {
		props := Properties{
			PropSubject: subject,
			"type":      msgType,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[PropCorrelationID] = corrID
		}
		return props
	}
}

// PropagateCorrelationWithTypeAndName returns a property transformation function
// that propagates the correlation ID, sets the message type, and uses the
// reflected type name of the output value as the subject.
//
// This is useful for command handlers where the output event type determines the subject.
//
// Example:
//
//	type OrderCreated struct { ... }
//
//	props := message.PropagateCorrelationWithTypeAndName[OrderCreated]("event")
//	// For an OrderCreated event, output properties will have:
//	// subject="OrderCreated", type="event", and correlation ID
func PropagateCorrelationWithTypeAndName[T any](msgType string) func(inProp Properties, out T) Properties {
	return func(inProp Properties, out T) Properties {
		props := Properties{
			PropSubject: typeNameOf[T](),
			"type":      msgType,
		}
		if corrID, ok := inProp.CorrelationID(); ok {
			props[PropCorrelationID] = corrID
		}
		return props
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
