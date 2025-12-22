package message

import (
	"context"

	"github.com/fxsml/gopipe/pipe"
)

// Pipe is a concrete type alias for a gopipe Pipe that processes Messages.
// type Pipe = pipe.Pipe[*Message, *Message]
type Pipe interface {
	pipe.Pipe[*Message, *Message]
	Match(attrs Attributes) bool
}

// NewPipe creates a message.Pipe from a pipe.Pipe and a matcher function.
func NewPipe(
	pipe pipe.Pipe[*Message, *Message],
	match func(attrs Attributes) bool,
) Pipe {
	return &messagePipe{
		pipe:  pipe,
		match: match,
	}
}

type messagePipe struct {
	pipe  pipe.Pipe[*Message, *Message]
	match func(attrs Attributes) bool
}

func (p *messagePipe) Pipe(ctx context.Context, msgs <-chan *Message) (<-chan *Message, error) {
	return p.pipe.Pipe(ctx, msgs)
}

func (p *messagePipe) Match(attrs Attributes) bool {
	return p.match(attrs)
}
