# Acking in Sagas: Complexity Analysis

## The Problem

When implementing sagas, we face a critical question: **When should the original message be acked?**

```go
// User sends command to broker
broker.Publish(CreateOrderCommand{...})

// Command handler processes
func handleCreateOrder(cmd CreateOrder) ([]OrderCreated, error) {
    saveOrder(cmd)
    return []OrderCreated{{...}}, nil
}

// ❓ When should we ack the CreateOrderCommand?
// A) When handler returns?
// B) When OrderCreated event is published?
// C) When saga completes (ShipOrder)?
// D) Never (each stage acks independently)?
```

## Option 1: Propagate Acking (Complex!)

### Approach

Copy acking from input to all output messages, update expected count:

```go
func NewCommandHandler[Cmd, Evt any](
    handle func(ctx, cmd Cmd) ([]Evt, error),
) Handler {
    return func(ctx, msg *Message) ([]*Message, error) {
        var cmd Cmd
        json.Unmarshal(msg.Payload, &cmd)

        events, err := handle(ctx, cmd)
        if err != nil {
            msg.Nack(err)
            return nil, err
        }

        // ✅ Update expected ack count for multistage
        if msg.a != nil {
            msg.a.SetExpectedAckCount(len(events))  // ❓ Need to make this updateable!
        }

        var outMsgs []*Message
        for _, evt := range events {
            payload, _ := json.Marshal(evt)

            outMsg := New(payload, props)

            // ❌ Copy acking to output
            outMsg.a = msg.a  // Share acking!

            outMsgs = append(outMsgs, outMsg)
        }

        // ❓ Don't ack here! Ack when ALL events are processed
        return outMsgs, nil
    }
}
```

### Flow Example

```
CreateOrder (acking: expected=1, count=0)
    ↓
handleCreateOrder produces 2 events
    ↓
Update acking: expected=2  // ❓ Need SetExpectedAckCount to be updateable!
    ↓
OrderCreated (acking: expected=2, count=0)
PaymentRequired (acking: expected=2, count=0)  // Same acking object!
    ↓
EmailHandler completes → acking.count++ (1/2)
SagaCoordinator completes → acking.count++ (2/2)
    ↓
✅ Original CreateOrder acked!
```

### Issues

1. **SetExpectedAckCount Complexity**
   ```go
   // Current implementation
   func (a *acking) SetExpectedAckCount(count int) {
       a.mu.Lock()
       defer a.mu.Unlock()

       if a.expectedAckCount != 1 {
           // ❌ Can't update if already set!
           return
       }

       a.expectedAckCount = count
   }

   // ❓ Need to change to:
   func (a *acking) SetExpectedAckCount(count int) {
       a.mu.Lock()
       defer a.mu.Unlock()

       // ❓ Allow updates until acked?
       if a.acked {
           return  // Can't update after ack
       }

       a.expectedAckCount = count  // ✅ Always updateable
   }

   // Or even:
   func (a *acking) IncreaseExpectedAckCount(delta int) {
       a.mu.Lock()
       defer a.mu.Unlock()

       if a.acked {
           return
       }

       a.expectedAckCount += delta  // ✅ Increment
   }
   ```

2. **Broker vs Internal Processing**
   ```go
   // If we publish to broker
   publisher.Send(ctx, "events", event)  // ✅ Sent to broker

   // ❓ When to ack?
   // A) When broker confirms? → What if saga fails later?
   // B) When saga completes? → Need to track!

   // If we process internally
   sagaHandler.Handle(event)  // ✅ Processed internally

   // ❓ Same acking object, but different semantics!
   ```

3. **Error Handling**
   ```go
   CreateOrder → OrderCreated → EmailHandler (fails!) ❌
                              → SagaCoordinator → ChargePayment

   // ❓ Should we nack the original CreateOrder?
   // - EmailHandler failed, but saga continued
   // - Is this a partial success or failure?
   ```

4. **Circular Acking**
   ```go
   // Saga feedback loop
   CreateOrder (acking=A)
       ↓
   OrderCreated (acking=A)
       ↓
   ChargePayment (acking=A)  // ❓ Same acking!
       ↓
   PaymentCharged (acking=A)
       ↓
   ShipOrder (acking=A)

   // ❓ When does acking=A complete?
   // - After ShipOrder?
   // - Need to track entire saga state!
   ```

### Pros
- ✅ End-to-end tracking
- ✅ Original message not acked until saga completes
- ✅ Can detect saga failures

### Cons
- ❌ **Very complex** acking logic
- ❌ Need to make SetExpectedAckCount updateable
- ❌ Different semantics for broker vs internal
- ❌ Hard to reason about when ack happens
- ❌ Circular dependencies in feedback loops
- ❌ Error handling is ambiguous

## Option 2: Independent Acking (Simple!)

### Approach

Each stage acks independently, don't propagate acking:

```go
func NewCommandHandler[Cmd, Evt any](
    handle func(ctx, cmd Cmd) ([]Evt, error),
) Handler {
    return func(ctx, msg *Message) ([]*Message, error) {
        var cmd Cmd
        json.Unmarshal(msg.Payload, &cmd)

        events, err := handle(ctx, cmd)
        if err != nil {
            msg.Nack(err)
            return nil, err
        }

        var outMsgs []*Message
        for _, evt := range events {
            payload, _ := json.Marshal(evt)

            // ✅ Create NEW message without acking
            outMsg := New(payload, props)  // No acking!

            outMsgs = append(outMsgs, outMsg)
        }

        // ✅ Ack immediately when handler completes
        msg.Ack()

        return outMsgs, nil
    }
}
```

### Flow Example

```
CreateOrder (acking=A from broker)
    ↓
handleCreateOrder completes
    ↓
✅ Ack CreateOrder immediately  // Done with this stage!
    ↓
Produce OrderCreated (no acking)  // Internal message
    ↓
EmailHandler processes OrderCreated
    ↓
(no acking needed - internal)
    ↓
SagaCoordinator produces ChargePayment (no acking)
    ↓
ChargePayment sent to broker → gets NEW acking (B)
    ↓
handleChargePayment completes
    ↓
✅ Ack ChargePayment (B) immediately
```

### Issues

1. **No End-to-End Tracking**
   ```go
   // CreateOrder acked immediately
   // But saga might fail later!

   CreateOrder ✅ Acked
       ↓
   OrderCreated (internal)
       ↓
   ChargePayment ❌ Fails!

   // ❌ CreateOrder already acked, can't retry entire saga
   ```

2. **Loss of Atomicity**
   ```go
   // Can't guarantee all-or-nothing semantics
   // Each stage is independent
   ```

### Pros
- ✅ **Very simple** acking logic
- ✅ Clear semantics: ack when stage completes
- ✅ No circular dependencies
- ✅ No need to update SetExpectedAckCount
- ✅ Broker vs internal has clear distinction

### Cons
- ❌ No end-to-end tracking via acking
- ❌ Can't retry entire saga if later stage fails
- ❌ Lost atomicity guarantees

## Option 3: Hybrid (Correlation IDs)

### Approach

Use independent acking + correlation IDs for tracking:

```go
// Each stage acks independently
msg.Ack()

// But propagate correlation ID for tracking
outMsg := New(payload, Properties{
    PropCorrelationID: msg.Properties.CorrelationID(),  // ✅ Track saga
})

// Monitor saga completion separately
type SagaTracker struct {
    sagas map[string]*SagaState
}

type SagaState struct {
    CorrelationID string
    Steps         []string  // ["OrderCreated", "PaymentCharged", ...]
    Status        string    // "running", "completed", "failed"
}

func (t *SagaTracker) RecordEvent(evt Event) {
    corrID := evt.CorrelationID
    state := t.sagas[corrID]

    state.Steps = append(state.Steps, evt.Name)

    if isTerminalEvent(evt) {
        state.Status = "completed"
        // ✅ Saga completed, log/metrics/monitoring
    }
}
```

### Pros
- ✅ Simple acking (each stage independent)
- ✅ End-to-end tracking via correlation IDs
- ✅ Clear separation: acking for reliability, correlation for tracking
- ✅ Can monitor saga health separately

### Cons
- ⚠️ Need separate saga tracker
- ⚠️ Can't automatically retry failed sagas

## Recommendation

**Use Option 3: Hybrid (Independent Acking + Correlation IDs)**

### Why?

1. **Acking Complexity is Too High**
   - Making SetExpectedAckCount updateable is complex
   - Circular dependencies in feedback loops
   - Ambiguous error handling
   - Different semantics for broker vs internal

2. **Acking is for Reliability, Not Business Logic**
   - Acking means: "I processed this message, don't redeliver"
   - It doesn't mean: "The entire saga completed"
   - Mixing acking with saga completion is conflating concerns

3. **Correlation IDs are Better for Tracking**
   - Industry standard (Zipkin, Jaeger, OpenTelemetry)
   - Clear semantics
   - Works across service boundaries
   - Doesn't interfere with message processing

### Implementation

```go
// Command handler
func NewCommandHandler[Cmd, Evt any](
    handle func(ctx, cmd Cmd) ([]Evt, error),
) Handler {
    return func(ctx, msg *Message) ([]*Message, error) {
        var cmd Cmd
        json.Unmarshal(msg.Payload, &cmd)

        events, err := handle(ctx, cmd)
        if err != nil {
            msg.Nack(err)
            return nil, err
        }

        var outMsgs []*Message
        for _, evt := range events {
            payload, _ := json.Marshal(evt)

            // ✅ Don't copy acking
            // ✅ DO copy correlation ID
            outMsg := New(payload, Properties{
                PropSubject:       reflect.TypeOf(evt).Name(),
                PropCorrelationID: msg.Properties.CorrelationID(),  // Track saga
                PropCreatedAt:     time.Now(),
                "type":            "event",
            })

            outMsgs = append(outMsgs, outMsg)
        }

        // ✅ Ack immediately when handler completes
        msg.Ack()

        return outMsgs, nil
    }
}

// Optional: Saga tracker
type SagaMonitor struct {
    sagas sync.Map  // corrID -> SagaState
}

func (m *SagaMonitor) OnEvent(evt Event) {
    corrID := evt.CorrelationID

    state, _ := m.sagas.LoadOrStore(corrID, &SagaState{
        CorrelationID: corrID,
        StartedAt:     time.Now(),
    })

    state.(*SagaState).RecordEvent(evt)

    if isTerminalEvent(evt) {
        log.Printf("✅ Saga %s completed", corrID)
        m.sagas.Delete(corrID)
    }
}
```

## Comparison Table

| Aspect | Propagate Acking | Independent Acking | Hybrid (Recommended) |
|--------|------------------|-------------------|---------------------|
| **Complexity** | ❌ Very High | ✅ Very Low | ✅ Low |
| **Acking Semantics** | ❌ Ambiguous | ✅ Clear | ✅ Clear |
| **End-to-End Tracking** | ✅ Via acking | ❌ None | ✅ Via correlation ID |
| **SetExpectedAckCount** | ❌ Need updateable | ✅ Not needed | ✅ Not needed |
| **Error Handling** | ❌ Ambiguous | ✅ Clear | ✅ Clear |
| **Saga Retry** | ✅ Possible | ❌ Not possible | ⚠️ Manual |
| **Broker vs Internal** | ❌ Conflated | ✅ Clear | ✅ Clear |
| **Industry Standard** | ❌ No | ⚠️ Partial | ✅ Yes (correlation IDs) |

## Testing Considerations

### What to Assert

```go
func TestCommandHandlerAcking(t *testing.T) {
    var ackCalled, nackCalled bool

    msg := NewWithAcking(
        commandPayload,
        props,
        func() { ackCalled = true },
        func(err error) { nackCalled = true },
    )

    handler := NewCommandHandler("CreateOrder", func(ctx, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{...}}, nil
    })

    outMsgs, err := handler.Handle(ctx, msg)

    // ✅ Assert: Input message was acked
    assert.True(t, ackCalled)
    assert.False(t, nackCalled)

    // ✅ Assert: Output messages DON'T have acking
    for _, outMsg := range outMsgs {
        assert.Nil(t, outMsg.a)  // No acking!
    }

    // ✅ Assert: Correlation ID propagated
    assert.Equal(t, msg.Properties.CorrelationID(), outMsgs[0].Properties.CorrelationID())
}
```

## Conclusion

**Recommendation: Don't propagate acking in sagas.**

**Instead:**
1. ✅ Each stage acks independently (simple, clear)
2. ✅ Propagate correlation IDs (end-to-end tracking)
3. ✅ Use saga monitor for visibility (optional)
4. ✅ Keep acking for reliability, not business logic

**Avoid:**
- ❌ Making SetExpectedAckCount updateable (complexity)
- ❌ Mixing acking semantics (broker vs internal)
- ❌ Using acking for saga completion (wrong abstraction)

This keeps gopipe simple and aligns with industry best practices (correlation IDs for tracing, acking for message reliability).
