package gopipe

import (
	"context"
	"errors"
	"maps"
)

// Metadata is a key-value store for additional information about pipeline items.
type Metadata map[string]any

// MetadataProvider is a function that provides Metadata for a given input.
type MetadataProvider[In any] func(in In) Metadata

// Args converts the metadata map into a slice of alternating keys and values,
// suitable for use with structured logging systems like slog.
func (m Metadata) Args() []any {
	args := make([]any, 0, len(m)*2)
	for k, v := range m {
		args = append(args, k, v)
	}
	return args
}

type metadataKeyType struct{}

var metadataKey = metadataKeyType{}

type metadataErrorWrapper struct {
	cause    error
	metadata Metadata
}

func (w *metadataErrorWrapper) Error() string {
	if w.cause != nil {
		return w.cause.Error()
	}
	return "gopipe metadata error"
}

func (w *metadataErrorWrapper) Unwrap() error {
	return w.cause
}

func newMetadataErrorWrapper(err error, metadata Metadata) error {
	return &metadataErrorWrapper{cause: err, metadata: metadata}
}

// UseMetadata creates middleware that attaches metadata to the processing context.
// The provided function produces metadata which may be extracted from the input value.
// Metadata is then available via MetadataFromContext or MetadataFromError.
func UseMetadata[In, Out any](m MetadataProvider[In]) MiddlewareFunc[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				metadata := MetadataFromContext(ctx)
				if metadata == nil {
					metadata = m(in)
				} else {
					maps.Copy(metadata, m(in))
				}
				ctx = context.WithValue(ctx, metadataKey, metadata)
				return next.Process(ctx, in)
			},
			func(in In, err error) {
				metadata := MetadataFromError(err)
				if metadata == nil {
					metadata = m(in)
				} else {
					maps.Copy(metadata, m(in))
				}
				err = newMetadataErrorWrapper(err, metadata)
				next.Cancel(in, err)
			})
	}
}

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

// MetadataFromError extracts metadata from an error.
// Returns nil if no metadata is present.
func MetadataFromError(err error) Metadata {
	if err == nil {
		return nil
	}
	var w *metadataErrorWrapper
	if errors.As(err, &w) {
		return w.metadata
	}
	return nil
}
