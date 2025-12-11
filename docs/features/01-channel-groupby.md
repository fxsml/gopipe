# Feature: Channel GroupBy

**Package:** `channel`
**Status:** ✅ Implemented
**Related ADRs:** [ADR 0016](../adr/0016-channel-package.md)

## Summary

Adds `GroupBy` function to aggregate items from a channel by key, emitting batches when size or time limits are reached. This is a prerequisite for efficient message batching in pub/sub operations.

## Implementation

```go
type Group[K comparable, V any] struct {
    Key   K
    Items []V
}

func GroupBy[K comparable, V any](
    in <-chan V,
    keyFunc func(V) K,
    config GroupByConfig,
) <-chan Group[K, V]
```

**Key features:**
- Batch by key with configurable size (`MaxBatchSize`) and time (`MaxDuration`) limits
- LRU eviction when concurrent groups exceed `MaxConcurrentGroups`
- Automatic flushing on channel close

## Usage Example

```go
msgs := make(chan *message.Message)
grouped := channel.GroupBy(msgs,
    func(m *message.Message) string {
        return m.Attributes["topic"].(string)
    },
    channel.GroupByConfig{
        MaxBatchSize: 100,
        MaxDuration:  time.Second,
    },
)
```

## Files Changed

- `channel/groupby.go` - Core implementation
- `channel/groupby_test.go` - Comprehensive tests

## Related Features

- [02-message-pubsub](02-message-pubsub.md) - Uses GroupBy for publisher batching
