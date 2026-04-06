package redis

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/alicebob/miniredis/v2"
)

func TestContextWithPipeline(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	t.Run("round trip", func(t *testing.T) {
		pipe := client.TxPipeline()
		ctx := ContextWithPipeline(context.Background(), pipe)

		got, ok := PipelineFromContext(ctx)
		if !ok {
			t.Fatal("expected pipeline in context")
		}
		if got != pipe {
			t.Fatal("expected same pipeline")
		}
	})

	t.Run("missing pipeline", func(t *testing.T) {
		_, ok := PipelineFromContext(context.Background())
		if ok {
			t.Fatal("expected no pipeline in empty context")
		}
	})
}

func TestCmdableFromContext(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	t.Run("returns pipeline when present", func(t *testing.T) {
		pipe := client.TxPipeline()
		ctx := ContextWithPipeline(context.Background(), pipe)

		cmd := CmdableFromContext(ctx, client)

		// Verify it's the pipeline by queuing a command
		cmd.Set(ctx, "key", "value", 0)
		// Pipeline: command is queued, not yet executed
		val := mr.Exists("key")
		if val {
			t.Fatal("expected key to not exist yet (pipeline not executed)")
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		if !mr.Exists("key") {
			t.Fatal("expected key to exist after exec")
		}
	})

	t.Run("returns fallback when no pipeline", func(t *testing.T) {
		ctx := context.Background()

		cmd := CmdableFromContext(ctx, client)

		// Verify it's the client by executing directly
		err := cmd.Set(ctx, "direct", "value", 0).Err()
		if err != nil {
			t.Fatalf("set: %v", err)
		}
		if !mr.Exists("direct") {
			t.Fatal("expected key to exist immediately (direct client)")
		}
	})
}
