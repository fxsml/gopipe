package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

type pipeAdapter[In, Out any] struct {
	pipe      pipe.Pipe[In, Out]
	marshal   pipe.Pipe[Out, *message.Message]
	unmarshal pipe.Pipe[*message.Message, In]
	match     message.Matcher
}

func (p *pipeAdapter[In, Out]) Start(ctx context.Context, msgs <-chan *message.Message) (<-chan *message.Message, error) {
	unmarshalOut, err := p.unmarshal.Start(ctx, msgs)
	if err != nil {
		return nil, err
	}
	pipeOut, err := p.pipe.Start(ctx, unmarshalOut)
	if err != nil {
		return nil, err
	}
	return p.marshal.Start(ctx, pipeOut)
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
	unmarshalFn := func(ctx context.Context, msg *message.Message) (In, error) {
		in := new(In)
		err := marshaler.Unmarshal(msg.Data, in)
		if err != nil {
			msg.Nack(err)
			return *in, err
		}
		msg.Ack()
		return *in, nil
	}

	marshalFn := func(ctx context.Context, out Out) (*message.Message, error) {
		data, err := marshaler.Marshal(out)
		if err != nil {
			return nil, err
		}
		msg := message.New(data, marshaler.Attributes(out))
		return msg, nil
	}

	unmarshal := pipe.NewProcessPipe(func(ctx context.Context, msg *message.Message) ([]In, error) {
		in, err := unmarshalFn(ctx, msg)
		if err != nil {
			return nil, err
		}
		return []In{in}, nil
	}, pipe.Config{})
	_ = unmarshal.ApplyMiddleware(
		middleware.Log[*message.Message, In](middleware.LogConfig{
			MessageFailure: "Failed to unmarshal message",
		}),
	)

	marshal := pipe.NewProcessPipe(func(ctx context.Context, out Out) ([]*message.Message, error) {
		msg, err := marshalFn(ctx, out)
		if err != nil {
			return nil, err
		}
		return []*message.Message{msg}, nil
	}, pipe.Config{})
	_ = marshal.ApplyMiddleware(
		middleware.Log[Out, *message.Message](middleware.LogConfig{
			MessageFailure: "Failed to marshal message",
		}),
	)

	return &pipeAdapter[In, Out]{
		pipe:      p,
		unmarshal: unmarshal,
		marshal:   marshal,
		match:     match,
	}
}
