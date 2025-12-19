package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/message"
)

type pipeAdapter[In, Out any] struct {
	pipe      pipe.Pipe[In, Out]
	marshal   pipe.Pipe[Out, *message.Message]
	unmarshal pipe.Pipe[*message.Message, In]
	match     message.Matcher
}

func (p *pipeAdapter[In, Out]) Start(ctx context.Context, msgs <-chan *message.Message) <-chan *message.Message {
	return p.marshal.Start(ctx, p.pipe.Start(ctx, p.unmarshal.Start(ctx, msgs)))
}

func (p *pipeAdapter[In, Out]) Match(attrs message.Attributes) bool {
	return p.match(attrs)
}

// NewCommandPipe creates a message.Pipe that wraps a pipe.Pipe[In, Out],
// using the provided CommandMarshaler for serialization and deserialization.
func NewCommandPipe[In, Out any](
	p pipe.Pipe[In, Out],
	match message.Matcher,
	marshaler CommandMarshaler,
) message.Pipe {
	unmarshal := pipe.NewTransformPipe(func(ctx context.Context, msg *message.Message) (In, error) {
		in := new(In)
		err := marshaler.Unmarshal(msg.Data, in)
		if err != nil {
			msg.Nack(err)
			return *in, err
		}
		msg.Ack()
		return *in, nil
	}, pipe.WithLogConfig[*message.Message, In](pipe.LogConfig{
		MessageFailure: "Failed to unmarshal message",
	}))
	marshal := pipe.NewTransformPipe(func(ctx context.Context, out Out) (*message.Message, error) {
		data, err := marshaler.Marshal(out)
		if err != nil {
			return nil, err
		}
		msg := message.New(data, marshaler.Attributes(out))
		return msg, nil
	}, pipe.WithLogConfig[Out, *message.Message](pipe.LogConfig{
		MessageFailure: "Failed to marshal message",
	}))
	return &pipeAdapter[In, Out]{
		pipe:      p,
		unmarshal: unmarshal,
		marshal:   marshal,
		match:     match,
	}
}
