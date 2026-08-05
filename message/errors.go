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

	// ErrDataNotRaw is returned when a message reaches UnmarshalPipe with
	// Data that isn't raw []byte.
	ErrDataNotRaw = errors.New("expected raw []byte data")

	// ErrDataNotTyped is returned when a message reaches MarshalPipe with
	// Data that is already raw []byte — marshaling it again would silently
	// double-encode.
	ErrDataNotTyped = errors.New("unexpected raw []byte data, want typed")
)
