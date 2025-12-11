package message

import (
	"context"

	"github.com/fxsml/gopipe"
)

// Pipe is a concrete type alias for a gopipe Pipe that processes Messages.
// type Pipe = gopipe.Pipe[*Message, *Message]
type Pipe interface {
	gopipe.Pipe[*Message, *Message]
	Match(attrs Attributes) bool
}

// NewPipe creates a message.Pipe from a gopipe.Pipe and a matcher function.
func NewPipe(
	pipe gopipe.Pipe[*Message, *Message],
	match func(attrs Attributes) bool,
) Pipe {
	return &messagePipe{
		pipe:  pipe,
		match: match,
	}
}

type messagePipe struct {
	pipe  gopipe.Pipe[*Message, *Message]
	match func(attrs Attributes) bool
}

func (p *messagePipe) Start(ctx context.Context, msgs <-chan *Message) <-chan *Message {
	return p.pipe.Start(ctx, msgs)
}

func (p *messagePipe) Match(attrs Attributes) bool {
	return p.match(attrs)
}
