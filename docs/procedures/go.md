# Go Procedures

## Pre-Push Checklist

```bash
make test   # Tests pass
make build  # Build succeeds
make vet    # No issues
```

## Godoc Standards

### DO

```go
// GroupBy aggregates items from the input channel by key, emitting batches
// when size or time limits are reached.
//
// For usage examples and patterns, see docs/features/01-channel-groupby.md.
func GroupBy[K comparable, V any](
    in <-chan V,
    keyFunc func(V) K,
    config GroupByConfig,
) <-chan Group[K, V]
```

### DON'T

```go
// GroupBy aggregates items. Example usage:
//
//    grouped := GroupBy(msgs, func(m Msg) string {
//        return m.Topic
//    }, GroupByConfig{MaxBatchSize: 100})
func GroupBy[K comparable, V any](...) <-chan Group[K, V]
```

### Guidelines

1. **Precise and concise** - No example implementations in godoc
2. **Link to docs** - Reference feature documentation for examples
3. **First sentence** - Starts with function name, describes what it does
4. **Parameters** - Document non-obvious parameters

## Deprecation

Mark deprecated code clearly:

```go
// Deprecated: Use NewTyped for pipelines or New for CloudEvents messages.
func OldFunction() {}
```

## Testing

- Tests in `*_test.go` files
- Table-driven tests preferred
- Use `t.Parallel()` where safe
- Mock external dependencies

## Error Handling

- Return errors, don't panic (except in `Must*` functions)
- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Check errors immediately after function call
