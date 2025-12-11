package cqrs

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

type pipeAdapter[In, Out any] struct {
	pipe      gopipe.Pipe[In, Out]
	marshal   gopipe.Pipe[Out, *message.Message]
	unmarshal gopipe.Pipe[*message.Message, In]
	match     message.Matcher
}

func (p *pipeAdapter[In, Out]) Start(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	return p.marshal.Start(ctx, p.pipe.Start(ctx, p.unmarshal.Start(ctx, msgs)))
}

func (p *pipeAdapter[In, Out]) Match(attrs message.Attributes) bool {
	return p.match(attrs)
}

// NewCommandPipe creates a message.Pipe that wraps a gopipe.Pipe[In, Out],
// using the provided CommandMarshaler for serialization and deserialization.
func NewCommandPipe[In, Out any](
	pipe gopipe.Pipe[In, Out],
	match message.Matcher,
	marshaler CommandMarshaler,
) message.Pipe {
	unmarshal := gopipe.NewTransformPipe(func(ctx context.Context, msg *message.Message) (In, error) {
		in := new(In)
		err := marshaler.Unmarshal(msg.Data, in)
		if err != nil {
			msg.Nack(err)
			return *in, err
		}
		msg.Ack()
		return *in, nil
	}, gopipe.WithLogConfig[*message.Message, In](gopipe.LogConfig{
		MessageFailure: "Failed to unmarshal message",
	}))
	marshal := gopipe.NewTransformPipe(func(ctx context.Context, out Out) (*message.Message, error) {
		data, err := marshaler.Marshal(out)
		if err != nil {
			return nil, err
		}
		msg := message.New(data, marshaler.Attributes(out))
		return msg, nil
	}, gopipe.WithLogConfig[Out, *message.Message](gopipe.LogConfig{
		MessageFailure: "Failed to marshal message",
	}))
	return &pipeAdapter[In, Out]{
		pipe:      pipe,
		unmarshal: unmarshal,
		marshal:   marshal,
		match:     match,
	}
}
