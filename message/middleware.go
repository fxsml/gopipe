package message

import "github.com/fxsml/gopipe/pipe"

type MiddlewareFunc = pipe.MiddlewareFunc[*Message, *Message]
