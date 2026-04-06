package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

type pipelineKey struct{}

// ContextWithPipeline returns a new context with the Redis pipeline attached.
func ContextWithPipeline(ctx context.Context, pipe goredis.Pipeliner) context.Context {
	return context.WithValue(ctx, pipelineKey{}, pipe)
}

// PipelineFromContext extracts the Redis pipeline from the context, if present.
func PipelineFromContext(ctx context.Context) (goredis.Pipeliner, bool) {
	pipe, ok := ctx.Value(pipelineKey{}).(goredis.Pipeliner)
	return pipe, ok
}

// CmdableFromContext returns the pipeline from context if available,
// otherwise returns the fallback client. Both satisfy redis.Cmdable,
// giving adapters a single API for queuing or executing commands.
func CmdableFromContext(ctx context.Context, fallback goredis.Cmdable) goredis.Cmdable {
	if pipe, ok := PipelineFromContext(ctx); ok {
		return pipe
	}
	return fallback
}
