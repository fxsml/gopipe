package cqrs

import (
	"context"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

type generatorAdapter[Out any] struct {
	generator gopipe.Generator[Out]
	marshal   gopipe.Pipe[Out, *message.Message]
}

func (g *generatorAdapter[Out]) Generate(ctx context.Context) <-chan *message.Message {
	return g.marshal.Start(ctx, g.generator.Generate(ctx))
}

func NewEventGenerator[Out any](
	generate func(context.Context) ([]Out, error),
	marshaler Marshaler,
) message.Generator {
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
	return &generatorAdapter[Out]{
		generator: gopipe.NewGenerator(generate),
		marshal:   marshal,
	}
}
