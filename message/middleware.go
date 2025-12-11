package message

import "github.com/fxsml/gopipe"

type MiddlewareFunc = gopipe.MiddlewareFunc[*Message, *Message]
