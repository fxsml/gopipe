package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

type generatorAdapter[Out any] struct {
	generator pipe.Generator[Out]
	marshal   pipe.Pipe[Out, *message.Message]
}

func (g *generatorAdapter[Out]) Generate(ctx context.Context) (<-chan *message.Message, error) {
	genOut, err := g.generator.Generate(ctx)
	if err != nil {
		return nil, err
	}
	return g.marshal.Start(ctx, genOut)
}

func NewEventGenerator[Out any](
	generate func(context.Context) ([]Out, error),
	marshaler Marshaler,
) message.Generator {
	marshalFn := func(ctx context.Context, out Out) (*message.Message, error) {
		data, err := marshaler.Marshal(out)
		if err != nil {
			return nil, err
		}
		msg := message.New(data, marshaler.Attributes(out))
		return msg, nil
	}

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

	return &generatorAdapter[Out]{
		generator: pipe.NewGenerator(generate, pipe.Config{}),
		marshal:   marshal,
	}
}
