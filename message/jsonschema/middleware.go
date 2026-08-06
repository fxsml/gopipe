package jsonschema

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipe/message"
)

// NewValidationMiddleware creates middleware for validating raw Message payloads.
// Use this for proxy scenarios where messages pass through without unmarshaling.
//
// Example - HTTP → AMQP proxy with validation:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{})
//	registry.MustRegister("order.created", orderSchema)
//
//	pipe := pipe.NewPassthroughPipe(cfg)
//	pipe.Use(jsonschema.NewValidationMiddleware(registry))
func NewValidationMiddleware(registry *Registry) message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			raw, ok := msg.Raw()
			if !ok {
				return nil, fmt.Errorf("%w: want raw []byte, got %T", message.ErrUnexpectedDataType, msg.Data)
			}
			// Validate before passing through
			if err := registry.Validate(msg.Type(), raw); err != nil {
				return nil, fmt.Errorf("validation failed: %w", err)
			}
			return next(ctx, msg)
		}
	}
}

// NewInputValidationMiddleware creates middleware for validating before unmarshaling.
// Use this with UnmarshalPipe to validate raw bytes before deserialization.
//
// Example - Validate before unmarshaling:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{
//	    Naming: message.DotNaming,
//	})
//	registry.MustRegisterType(CreateOrder{}, schema)
//
//	marshaler := message.NewJSONMarshaler()
//	unmarshalPipe := message.NewUnmarshalPipe(registry, marshaler, cfg)
//	unmarshalPipe.Use(jsonschema.NewInputValidationMiddleware(registry))
func NewInputValidationMiddleware(registry *Registry) message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			raw, ok := msg.Raw()
			if !ok {
				return nil, fmt.Errorf("%w: want raw []byte, got %T", message.ErrUnexpectedDataType, msg.Data)
			}
			// Validate BEFORE unmarshaling (fail fast)
			if err := registry.Validate(msg.Type(), raw); err != nil {
				return nil, fmt.Errorf("input validation failed: %w", err)
			}
			return next(ctx, msg)
		}
	}
}

// NewOutputValidationMiddleware creates middleware for validating after marshaling.
// Use this with MarshalPipe to validate marshaled bytes before sending.
//
// Example - Validate after marshaling:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{
//	    Naming: message.DotNaming,
//	})
//	registry.MustRegisterType(OrderCreated{}, schema)
//
//	marshaler := message.NewJSONMarshaler()
//	marshalPipe := message.NewMarshalPipe(marshaler, cfg)
//	marshalPipe.Use(jsonschema.NewOutputValidationMiddleware(registry))
func NewOutputValidationMiddleware(registry *Registry) message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Marshal first
			results, err := next(ctx, msg)
			if err != nil {
				return nil, err
			}

			// Validate AFTER marshaling
			for _, out := range results {
				raw, ok := out.Raw()
				if !ok {
					return nil, fmt.Errorf("%w: want raw []byte, got %T", message.ErrUnexpectedDataType, out.Data)
				}
				if err := registry.Validate(out.Type(), raw); err != nil {
					return nil, fmt.Errorf("output validation failed: %w", err)
				}
			}

			return results, nil
		}
	}
}
