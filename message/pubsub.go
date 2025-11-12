package message

import (
	"context"
	"encoding/json"

	"github.com/fxsml/gopipe"
)

type Publisher interface {
	Publish(messages <-chan *Message[[]byte]) (<-chan struct{}, error)
}

type Subscriber interface {
	Subscribe(ctx context.Context) (<-chan *Message[[]byte], error)
}

type PubSub interface {
	Start(ctx context.Context, topic string) (<-chan struct{}, error)
}

type pubSub[In, Out any] struct {
	pipe gopipe.Pipe[*Message[In], *Message[Out]]
	pub  Publisher
	sub  Subscriber
}

func (ps *pubSub[In, Out]) Start(ctx context.Context, topic string) (<-chan struct{}, error) {
	ctx, cancel := context.WithCancel(ctx)

	inBytes, err := ps.sub.Subscribe(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	in := gopipe.NewTransformPipe(func(ctx context.Context, msg *Message[[]byte]) (*Message[In], error) {
		var payload In
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return nil, err
		}
		return CopyMessage(msg, payload), nil
	}).Start(ctx, inBytes)

	out := ps.pipe.Start(ctx, in)

	outBytes := gopipe.NewTransformPipe(func(ctx context.Context, msg *Message[Out]) (*Message[[]byte], error) {
		payload, err := json.Marshal(msg.Payload)
		if err != nil {
			return nil, err
		}
		return CopyMessage(msg, payload), nil
	}).Start(ctx, out)

	done, err := ps.pub.Publish(outBytes)
	if err != nil {
		cancel()
		return nil, err
	}

	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		cancel()
	}()

	return done, nil
}

func NewPubSub[In, Out any](
	pipe gopipe.Pipe[*Message[In], *Message[Out]],
	sub Subscriber,
	pub Publisher,
) PubSub {
	return &pubSub[In, Out]{
		pipe: pipe,
		pub:  pub,
		sub:  sub,
	}
}
