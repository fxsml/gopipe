package cqrs

import (
	"fmt"

	"github.com/fxsml/gopipe/message"
)

var (
	// ErrInvalidMessagePayload indicates message payload marshal or unmarshal failure.
	ErrInvalidMessagePayload = fmt.Errorf("invalid message payload")
)

// RouterConfig is an alias for message.RouterConfig for backward compatibility.
type RouterConfig = message.RouterConfig

// Router is an alias for message.Router for backward compatibility.
type Router = message.Router

// NewRouter is an alias for message.NewRouter for backward compatibility.
var NewRouter = message.NewRouter
