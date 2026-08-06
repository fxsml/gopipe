package message

import "errors"

// ErrorHandler processes errors from Router, Merger, and Distributor.
type ErrorHandler func(msg *Message, err error)

var (
	// ErrAlreadyStarted is returned when Pipe() is called on a router that has already started.
	ErrAlreadyStarted = errors.New("router already started")

	// ErrNoHandler is returned when no handler exists for a message type.
	ErrNoHandler = errors.New("no handler for message type")

	// ErrHandlerRejected is returned when a message is rejected by handler matcher.
	ErrHandlerRejected = errors.New("message rejected by handler matcher")

	// ErrUnknownType is returned when unmarshaling a message with unknown type.
	ErrUnknownType = errors.New("unknown message type")

	// ErrHandlerExists is returned when registering a handler for an event type that already has one.
	ErrHandlerExists = errors.New("handler already registered for event type")

	// ErrCommandDataMismatch is returned when a commandHandler's message data is nil
	// or does not type-assert to the expected command type.
	ErrCommandDataMismatch = errors.New("command data type mismatch")

	// ErrUnexpectedDataType is returned when a message reaches a pipeline
	// stage with Data in the wrong state for that stage — raw when typed
	// was expected, or already-typed when raw was expected. This is always
	// a pipeline composition bug (wrong stage order, wrong channel wired
	// in, double marshal/unmarshal), never a property of the message's
	// actual payload — bad wire input fails unmarshaling itself, it never
	// produces a typed Go value in Data. Wrapped with directional context
	// (want/got) at each call site.
	ErrUnexpectedDataType = errors.New("unexpected data type")
)
