# Ack/Nack Pattern Evaluation

## Overview

This document evaluates the ack/nack pattern introduced in the `message` package and provides guidance on when and how to use it within the gopipe framework.

## Current Implementation

### Message Structure (message/gopipe.go)

The `Message[T]` type wraps a payload with acknowledgment capabilities:

```go
type Message[T any] struct {
    ID       string
    Metadata gopipe.Metadata
    Payload  T
    deadline time.Time

    ack  func()
    nack func(error)
    ackType ackType
}
```

Key features:
- **Thread-safe**: Uses mutex to prevent double ack/nack
- **Mutually exclusive**: Once acked, cannot be nacked, and vice versa
- **Optional**: Both ack and nack functions can be nil
- **Returns bool**: Indicates whether the operation succeeded

### Integration with Processor

The gopipe `Processor` interface has two methods:
- `Process(context.Context, In) ([]Out, error)` - Transforms input to output
- `Cancel(In, error)` - Handles failures

Currently in `processor.go:130-136`, when processing fails:
```go
if res, err := proc.Process(ctx, val); err != nil {
    proc.Cancel(val, newErrFailure(err))
} else {
    for _, r := range res {
        out <- r
    }
}
```

## Design Decision: When to Ack/Nack

### ❌ Anti-Pattern: Manual Ack/Nack in User Code

**DO NOT** require users to manually call ack/nack in their handler functions:

```go
// BAD: User has to remember to ack/nack
handle := func(ctx context.Context, msg *Message[Data]) ([]Result, error) {
    result, err := process(msg.Payload)
    if err != nil {
        msg.Nack(err)  // User must remember this
        return nil, err
    }
    msg.Ack()  // User must remember this
    return []Result{result}, nil
}
```

**Problems:**
1. Easy to forget to call ack/nack
2. Mixes business logic with infrastructure concerns
3. Inconsistent - some paths might not ack/nack
4. Error-prone when dealing with panics or early returns

### ✅ Recommended: Automatic Ack/Nack Based on Outcome

The framework should **automatically** ack/nack based on the result:

```go
// GOOD: Framework handles ack/nack automatically
handle := func(ctx context.Context, data Data) (Result, error) {
    // User only writes business logic
    return process(data)
}
```

**When to Ack:**
- Process returns successfully (no error)
- All outputs have been successfully sent to the output channel

**When to Nack:**
- Process returns an error
- Context is canceled before processing completes
- Panic during processing (with recover middleware)

## Implementation Recommendation

### Option 1: Integrate with Cancel Method (RECOMMENDED)

Update `gopipe.NewProcessPipe` to use `WithCancel` option:

```go
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    opts ...gopipe.Option[*Message[In], *Message[Out]],
) gopipe.Pipe[*Message[In], *Message[Out]] {

    // Add cancel handler that calls Nack
    opts = append([]gopipe.Option[*Message[In], *Message[Out]]{
        gopipe.WithCancel(func(msg *Message[In], err error) {
            msg.Nack(err)
        }),
    }, opts...)

    // ... rest of implementation

    return gopipe.NewProcessPipe(
        func(ctx context.Context, msg *Message[In]) ([]*Message[Out], error) {
            // Process with deadline
            if msg.deadline != (time.Time{}) {
                var cancel context.CancelFunc
                ctx, cancel = context.WithDeadline(ctx, msg.deadline)
                defer cancel()
            }

            results, err := handle(ctx, msg.Payload)
            if err != nil {
                // Don't nack here - let Cancel handle it
                return nil, err
            }

            // Success - Ack the message
            msg.Ack()

            // Create output messages
            var messages []*Message[Out]
            for _, result := range results {
                messages = append(messages, NewMessage("", result))
            }
            return messages, nil
        },
        opts...,
    )
}
```

**Advantages:**
- Leverages existing infrastructure (Cancel method)
- Consistent with gopipe patterns
- Handles all error cases (processing errors, context cancellation)
- No changes needed to core gopipe

### Option 2: Propagate Ack/Nack to Output Messages

Create a chain where output messages inherit ack/nack:

```go
// Output message Acks only when all derived processing succeeds
outMsg := NewMessageWithAck(
    id,
    payload,
    func() { inputMsg.Ack() },  // Propagate ack
    func(err error) { inputMsg.Nack(err) },  // Propagate nack
)
```

**Problems:**
- Complex: Who owns the ack? What if one input produces multiple outputs?
- Timing: When to ack? After first output processed or all outputs processed?
- Not suitable for 1-to-many transformations
- Breaks down with batching, filtering, or routing

**Use Case:** Only useful for simple 1-to-1 transformations where each output directly corresponds to one input.

## Ack/Nack Semantics

### At-Least-Once Processing

Ack/Nack enables at-least-once delivery guarantees:

1. **Nack** → Message broker redelivers the message
2. **Ack** → Message broker removes message from queue
3. **No Ack/Nack** → Message broker redelivers after timeout

### Idempotency Requirement

Since messages may be redelivered, handlers must be **idempotent**:
- Same input produces same output
- Safe to execute multiple times
- Use message ID for deduplication if needed

## When NOT to Use Ack/Nack

Don't use ack/nack for:
1. **In-memory channels** - No persistence, no redelivery needed
2. **Fire-and-forget operations** - Don't care about success/failure
3. **Read-only operations** - No side effects to confirm
4. **Internal pipeline stages** - Use regular error handling

## Summary

**Recommended Approach:**

1. ✅ **Automatic**: Framework calls ack/nack based on Process result
2. ✅ **Use Cancel method**: Integrate nack with existing Cancel infrastructure
3. ✅ **Call Ack on success**: After successful processing, before returning outputs
4. ✅ **No propagation**: Don't propagate ack/nack to output messages (too complex)
5. ✅ **User writes business logic only**: No ack/nack in user code

**Decision Rule:**
```
Process succeeds → Ack()
Process fails    → Cancel() → Nack(err)
Context canceled → Cancel() → Nack(err)
```

This keeps the framework clean, automatic, and prevents user errors.
