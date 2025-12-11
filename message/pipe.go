package message

import "github.com/fxsml/gopipe"

// Pipe is a concrete type alias for a gopipe Pipe that processes Messages.
type Pipe = gopipe.Pipe[*Message, *Message]
