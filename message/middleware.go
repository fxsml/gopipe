package message

import "github.com/fxsml/gopipe/pipe/middleware"

// Middleware wraps a message ProcessFunc with additional behavior.
type Middleware = middleware.Middleware[*Message, *Message]
