package middleware

import (
	"context"
	"errors"

	"github.com/fxsml/gopipe"
)

// Metadata is a key-value store for additional information about pipeline items.
type Metadata map[string]any

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
func UseMetadata[In, Out any](m func(in In) Metadata) MiddlewareFunc[In, Out] {
	return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
		return gopipe.NewProcessor(
			func(ctx context.Context, in In) (Out, error) {
				metadata := m(in)
				ctx = context.WithValue(ctx, metadataKey, metadata)
				out, err := next.Process(ctx, in)
				if err != nil {
					err = newMetadataErrorWrapper(err, metadata)
				}
				return out, err
			},
			func(in In, err error) {
				if gopipe.IsCancel(err) {
					metadata := m(in)
					err = newMetadataErrorWrapper(err, metadata)
				}
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
