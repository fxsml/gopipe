package jsonschema

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// NewValidationMiddleware creates middleware for validating RawMessage payloads.
// Use this for proxy scenarios where messages pass through without unmarshaling.
//
// Example - HTTP → AMQP proxy with validation:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{})
//	registry.MustRegister("order.created", orderSchema)
//
//	pipe := pipe.NewPassthroughPipe(cfg)
//	pipe.Use(jsonschema.NewValidationMiddleware(registry))
func NewValidationMiddleware(registry *Registry) middleware.Middleware[*message.RawMessage, *message.RawMessage] {
	return func(next middleware.ProcessFunc[*message.RawMessage, *message.RawMessage]) middleware.ProcessFunc[*message.RawMessage, *message.RawMessage] {
		return func(ctx context.Context, raw *message.RawMessage) ([]*message.RawMessage, error) {
			// Validate before passing through
			if err := registry.Validate(raw.Type(), raw.Data); err != nil {
				return nil, fmt.Errorf("validation failed: %w", err)
			}
			return next(ctx, raw)
		}
	}
}

// NewInputValidationMiddleware creates middleware for validating before unmarshaling.
// Use this with UnmarshalPipe to validate raw bytes before deserialization.
//
// Example - Validate before unmarshaling:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{
//	    Naming: message.KebabNaming,
//	})
//	registry.MustRegisterType(CreateOrder{}, schema)
//
//	marshaler := message.NewJSONMarshaler()
//	unmarshalPipe := message.NewUnmarshalPipe(registry, marshaler, cfg)
//	unmarshalPipe.Use(jsonschema.NewInputValidationMiddleware(registry))
func NewInputValidationMiddleware(registry *Registry) middleware.Middleware[*message.RawMessage, *message.Message] {
	return func(next middleware.ProcessFunc[*message.RawMessage, *message.Message]) middleware.ProcessFunc[*message.RawMessage, *message.Message] {
		return func(ctx context.Context, raw *message.RawMessage) ([]*message.Message, error) {
			// Validate BEFORE unmarshaling (fail fast)
			if err := registry.Validate(raw.Type(), raw.Data); err != nil {
				return nil, fmt.Errorf("input validation failed: %w", err)
			}
			return next(ctx, raw)
		}
	}
}

// NewOutputValidationMiddleware creates middleware for validating after marshaling.
// Use this with MarshalPipe to validate marshaled bytes before sending.
//
// Example - Validate after marshaling:
//
//	registry := jsonschema.NewRegistry(jsonschema.Config{
//	    Naming: message.KebabNaming,
//	})
//	registry.MustRegisterType(OrderCreated{}, schema)
//
//	marshaler := message.NewJSONMarshaler()
//	marshalPipe := message.NewMarshalPipe(marshaler, cfg)
//	marshalPipe.Use(jsonschema.NewOutputValidationMiddleware(registry))
func NewOutputValidationMiddleware(registry *Registry) middleware.Middleware[*message.Message, *message.RawMessage] {
	return func(next middleware.ProcessFunc[*message.Message, *message.RawMessage]) middleware.ProcessFunc[*message.Message, *message.RawMessage] {
		return func(ctx context.Context, msg *message.Message) ([]*message.RawMessage, error) {
			// Marshal first
			results, err := next(ctx, msg)
			if err != nil {
				return nil, err
			}

			// Validate AFTER marshaling
			for _, raw := range results {
				if err := registry.Validate(raw.Type(), raw.Data); err != nil {
					return nil, fmt.Errorf("output validation failed: %w", err)
				}
			}

			return results, nil
		}
	}
}
