package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
)

func NewProcessPipe[In, Out any](
	handle func(context.Context, In) ([]Out, error),
	opts ...gopipe.Option[*Message[In], *Message[Out]],
) gopipe.Pipe[*Message[In], *Message[Out]] {
	opts = append(opts, gopipe.WithMetadataProvider[*Message[In], *Message[Out]](
		func(msg *Message[In]) gopipe.Metadata {
			return msg.Metadata
		},
	))

	return gopipe.NewProcessPipe(
		func(ctx context.Context, msg *Message[In]) ([]*Message[Out], error) {
			if msg.deadline != (time.Time{}) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, msg.deadline)
				defer cancel()
			}

			results, err := handle(ctx, msg.Payload)
			if err != nil {
				return nil, err
			}

			var messages []*Message[Out]
			for _, result := range results {
				messages = append(messages, &Message[Out]{Payload: result})
			}
			return messages, nil
		},
		opts...,
	)
}
