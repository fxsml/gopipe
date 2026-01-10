package message

import "errors"

var (
	// ErrAlreadyStarted is returned when Start() is called on a running engine.
	ErrAlreadyStarted = errors.New("engine already started")

	// ErrInputRejected is returned when a message is rejected by input matcher.
	ErrInputRejected = errors.New("message rejected by input matcher")

	// ErrNoHandler is returned when no handler exists for a CE type.
	ErrNoHandler = errors.New("no handler for CE type")

	// ErrHandlerRejected is returned when a message is rejected by handler matcher.
	ErrHandlerRejected = errors.New("message rejected by handler matcher")

	// ErrUnknownType is returned when unmarshaling a message with unknown CE type.
	ErrUnknownType = errors.New("unknown CE type")
)
