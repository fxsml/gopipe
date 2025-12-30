package message

import (
	"context"

	"github.com/fxsml/gopipe/pipe"
)

// NewUnmarshalPipe creates a pipe that unmarshals RawMessage to Message.
// Uses registry to create typed instances for unmarshaling.
// Returns error via cfg.ErrorHandler if type not in registry or unmarshal fails.
func NewUnmarshalPipe(registry TypeRegistry, marshaler Marshaler, cfg pipe.Config) *pipe.ProcessPipe[*RawMessage, *Message] {
	return pipe.NewProcessPipe(func(ctx context.Context, raw *RawMessage) ([]*Message, error) {
		ceType, _ := raw.Attributes["type"].(string)

		instance := registry.NewInstance(ceType)
		if instance == nil {
			return nil, ErrUnknownType
		}

		if err := marshaler.Unmarshal(raw.Data, instance); err != nil {
			return nil, err
		}

		return []*Message{{
			Data:       instance,
			Attributes: raw.Attributes,
			a:          raw.a,
		}}, nil
	}, cfg)
}

// NewMarshalPipe creates a pipe that marshals Message to RawMessage.
// No registry needed - marshals msg.Data directly.
func NewMarshalPipe(marshaler Marshaler, cfg pipe.Config) *pipe.ProcessPipe[*Message, *RawMessage] {
	return pipe.NewProcessPipe(func(ctx context.Context, msg *Message) ([]*RawMessage, error) {
		data, err := marshaler.Marshal(msg.Data)
		if err != nil {
			return nil, err
		}

		attrs := msg.Attributes
		if attrs == nil {
			attrs = make(Attributes)
		}
		attrs["datacontenttype"] = marshaler.DataContentType()

		return []*RawMessage{{
			Data:       data,
			Attributes: attrs,
			a:          msg.a,
		}}, nil
	}, cfg)
}
