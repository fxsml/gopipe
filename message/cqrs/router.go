package cqrs

import "fmt"

var (
	// ErrInvalidMessagePayload indicates message payload marshal or unmarshal failure.
	ErrInvalidMessagePayload = fmt.Errorf("invalid message payload")
)
