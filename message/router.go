package message

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
)

var (
	ErrInvalidMessageProperties = fmt.Errorf("invalid message properties")
	ErrInvalidMessagePayload    = fmt.Errorf("invalid message payload")
)

type RouterConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *gopipe.RetryConfig
	Recover     bool

	Marshal   func(msg any) ([]byte, error)
	Unmarshal func(data []byte, msg any) error
}

type Router struct {
	handlers []*Handler
	config   RouterConfig
}

func NewRouter(config RouterConfig, handlers ...*Handler) *Router {
	return &Router{
		handlers: handlers,
		config:   config,
	}
}

func (r *Router) AddHandler(handler *Handler) {
	r.handlers = append(r.handlers, handler)
}

func (r *Router) Start(ctx context.Context, msgs <-chan *Message) <-chan *Message {
	for _, h := range r.handlers {
		if r.config.Marshal != nil {
			h.marshal = r.config.Marshal
		}
		if r.config.Unmarshal != nil {
			h.unmarshal = r.config.Unmarshal
		}
	}

	handle := func(ctx context.Context, msg *Message) ([]*Message, error) {
		for _, h := range r.handlers {
			if h.Match(msg.Properties) {
				return h.Handle(ctx, msg)
			}
		}
		err := fmt.Errorf("%w: no handler matched", ErrInvalidMessageProperties)
		msg.Nack(err)
		return nil, err
	}

	opts := []gopipe.Option[*Message, *Message]{
		gopipe.WithLogConfig[*Message, *Message](gopipe.LogConfig{
			MessageSuccess: "Processed messages",
			MessageFailure: "Failed to process messages",
			MessageCancel:  "Canceled processing messages",
		}),
		gopipe.WithMetadataProvider[*Message, *Message](func(msg *Message) gopipe.Metadata {
			metadata := gopipe.Metadata{}
			if id, ok := IDProps(msg.Properties); ok {
				metadata["message_id"] = id
			}
			if corr, ok := CorrelationIDProps(msg.Properties); ok {
				metadata["correlation_id"] = corr
			}
			return metadata
		}),
	}
	if r.config.Recover {
		opts = append(opts, gopipe.WithRecover[*Message, *Message]())
	}
	if r.config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[*Message, *Message](r.config.Concurrency))
	}
	if r.config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[*Message, *Message](r.config.Timeout))
	}
	if r.config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[*Message, *Message](*r.config.Retry))
	}

	return gopipe.NewProcessPipe(handle, opts...).Start(ctx, msgs)
}

type Handler struct {
	Handle func(ctx context.Context, msg *Message) ([]*Message, error)
	Match  func(prop map[string]any) bool

	marshal   func(msg any) ([]byte, error)
	unmarshal func(data []byte, msg any) error
}

func NewHandler[In, Out any](
	handle func(ctx context.Context, payload In) ([]Out, error),
	match func(prop map[string]any) bool,
	props func(prop map[string]any) map[string]any,
) *Handler {
	h := &Handler{
		Match:     match,
		unmarshal: json.Unmarshal,
		marshal:   json.Marshal,
	}

	h.Handle = func(ctx context.Context, msg *Message) ([]*Message, error) {
		var payload In
		if err := h.unmarshal(msg.Payload, &payload); err != nil {
			err = fmt.Errorf("unmarshal: %w: %w", ErrInvalidMessagePayload, err)
			msg.Nack(err)
			return nil, err
		}

		out, err := handle(ctx, payload)
		if err != nil {
			err = fmt.Errorf("handle message: %w", err)
			msg.Nack(err)
			return nil, err
		}

		var msgs []*Message
		for _, o := range out {
			data, err := h.marshal(o)
			if err != nil {
				err = fmt.Errorf("marshal: %w: %w", ErrInvalidMessagePayload, err)
				msg.Nack(err)
				return nil, err
			}
			outMsg := Copy(msg, data)
			outMsg.Properties = props(msg.Properties)
			msgs = append(msgs, outMsg)
		}

		msg.Ack()
		return msgs, nil
	}
	return h
}
