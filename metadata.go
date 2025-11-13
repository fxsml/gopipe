package gopipe

import (
	"context"
	"errors"
)

// Metadata is a key-value store for additional information about pipeline items.
type Metadata map[string]any

// MetadataProvider is a function that provides Metadata for a processing context.
// It may extract information from the input value.
type MetadataProvider[In any] func(in In, metadata Metadata)

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

// WithMetadataProvider adds a metadata provider to enrich context with metadata
// for each input. Can be used multiple times to add multiple providers.
// Metadata is available via MetadataFromContext or MetadataFromError.
// Metadata is used in logging and metrics collection.
func WithMetadataProvider[In, Out any](provider MetadataProvider[In]) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.metadataProvider = append(cfg.metadataProvider, useMetadata[In, Out](provider))
	}
}

func (m Metadata) args() []any {
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

func useMetadata[In, Out any](m MetadataProvider[In]) MiddlewareFunc[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				metadata := MetadataFromContext(ctx)
				if metadata == nil {
					metadata = make(Metadata)
				}
				m(in, metadata)
				ctx = context.WithValue(ctx, metadataKey, metadata)
				return next.Process(ctx, in)
			},
			func(in In, err error) {
				metadata := MetadataFromError(err)
				if metadata == nil {
					metadata = make(Metadata)
				}
				m(in, metadata)
				err = newMetadataErrorWrapper(err, metadata)
				next.Cancel(in, err)
			})
	}
}
