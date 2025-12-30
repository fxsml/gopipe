package message

import "errors"

var (
	// ErrAlreadyStarted is returned when Start() is called on a running engine.
	ErrAlreadyStarted = errors.New("engine already started")

	// ErrInputRejected is returned when a message is rejected by input matcher.
	ErrInputRejected = errors.New("message rejected by input matcher")

	// ErrNoMatchingOutput is returned when no output matches the message.
	ErrNoMatchingOutput = errors.New("no output matches message type")

	// ErrNoHandler is returned when no handler exists for a CE type.
	ErrNoHandler = errors.New("no handler for CE type")

	// ErrHandlerRejected is returned when a message is rejected by handler matcher.
	ErrHandlerRejected = errors.New("message rejected by handler matcher")

	// ErrNotAHandler is returned when Register is called with a TypeEntry that
	// doesn't also implement Handler.
	ErrNotAHandler = errors.New("type entry does not implement Handler")
)
