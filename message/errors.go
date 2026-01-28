package message

import "errors"

var (
	// ErrAlreadyStarted is returned when Start() is called on a running engine.
	ErrAlreadyStarted = errors.New("engine already started")

	// ErrInputRejected is returned when a message is rejected by input matcher.
	ErrInputRejected = errors.New("message rejected by input matcher")

	// ErrNoHandler is returned when no handler exists for a message type.
	ErrNoHandler = errors.New("no handler for message type")

	// ErrHandlerRejected is returned when a message is rejected by handler matcher.
	ErrHandlerRejected = errors.New("message rejected by handler matcher")

	// ErrUnknownType is returned when unmarshaling a message with unknown type.
	ErrUnknownType = errors.New("unknown message type")

	// ErrHandlerExists is returned when registering a handler for an event type that already has one.
	ErrHandlerExists = errors.New("handler already registered for event type")
)
