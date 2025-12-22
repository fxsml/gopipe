package middleware

import (
	"context"
	"maps"
)

// Metadata is a key-value store for additional information about pipeline items.
type Metadata map[string]any

// MetadataFromContext extracts metadata from a context.
// Returns nil if no metadata is present.
func MetadataFromContext(ctx context.Context) Metadata {
	if ctx == nil {
		return nil
	}
	if metadata, ok := ctx.Value(metadataKey).(Metadata); ok {
		return metadata
	}
	return nil
}

// Args converts metadata to a flat key-value slice for logging.
func (m Metadata) Args() []any {
	args := make([]any, 0, len(m)*2)
	for k, v := range m {
		args = append(args, k, v)
	}
	return args
}

type metadataKeyType struct{}

var metadataKey = metadataKeyType{}

// MetadataProvider wraps a ProcessFunc with metadata enrichment.
// It attaches metadata from the provider to the context for downstream use.
func MetadataProvider[In, Out any](provider func(in In) Metadata) Middleware[In, Out] {
	return func(next ProcessFunc[In, Out]) ProcessFunc[In, Out] {
		return func(ctx context.Context, in In) ([]Out, error) {
			metadata := MetadataFromContext(ctx)
			if metadata == nil {
				metadata = provider(in)
			} else {
				maps.Copy(metadata, provider(in))
			}
			ctx = context.WithValue(ctx, metadataKey, metadata)
			return next(ctx, in)
		}
	}
}
