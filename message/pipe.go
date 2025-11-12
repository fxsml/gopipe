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
	// Prepend framework options before user options
	opts = append([]gopipe.Option[*Message[In], *Message[Out]]{
		// Automatically nack on any failure (processing error or context cancellation)
		gopipe.WithCancel[*Message[In], *Message[Out]](func(msg *Message[In], err error) {
			msg.Nack(err)
		}),
		// Propagate metadata from input message
		gopipe.WithMetadataProvider[*Message[In], *Message[Out]](
			func(msg *Message[In]) gopipe.Metadata {
				return msg.Metadata
			},
		),
	}, opts...)

	return gopipe.NewProcessPipe(
		func(ctx context.Context, msg *Message[In]) ([]*Message[Out], error) {
			// Apply message deadline if set
			if msg.deadline != (time.Time{}) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, msg.deadline)
				defer cancel()
			}

			// Process the payload with user's handler
			results, err := handle(ctx, msg.Payload)
			if err != nil {
				// Don't nack here - WithCancel handles it automatically
				return nil, err
			}

			// Success - acknowledge the input message
			msg.Ack()

			// Create output messages from results
			var messages []*Message[Out]
			for _, result := range results {
				messages = append(messages, NewMessage(generateID(), result))
			}
			return messages, nil
		},
		opts...,
	)
}
