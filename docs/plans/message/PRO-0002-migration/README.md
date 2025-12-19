# PRO-0002: Migration from gopipe to goengine

**Status:** Proposed
**Priority:** High
**Depends On:** PRO-0001-repo-init

## Overview

This plan covers migrating CloudEvents messaging code from gopipe to the new goengine repository.

## What Moves to goengine

### Packages to Move

| gopipe Package | goengine Package | Notes |
|----------------|------------------|-------|
| `message/` | `message/` | Core message types |
| `message/broker/` | `adapters/` | Broker implementations |
| `message/cqrs/` | `cqrs/` | CQRS handlers |
| `message/multiplex/` | `router/` | Topic routing |
| `message/cloudevents/` | `message/` | Merge into message |

### Code Changes During Migration

1. **Message Type**
   - Add mandatory CloudEvents validation
   - Remove generics from Message (PRO-0005)
   - Add Type Registry

2. **Broker Adapters**
   - Rename from `broker/` to `adapters/`
   - Remove Sender/Receiver interfaces
   - Implement as Subscriber[*Message]

3. **CQRS Package**
   - Keep type-safe command/event handlers
   - Update to use new Message type

## Migration Steps

### Phase 1: Code Migration

```bash
# In goengine repo
# Copy message package
cp -r ../gopipe/message/* message/

# Rename broker to adapters
mv message/broker adapters/

# Move CQRS
mv message/cqrs cqrs/

# Move multiplex to router
mv message/multiplex router/
```

### Phase 2: API Changes

1. **Update imports**
   ```go
   // Before
   import "github.com/fxsml/gopipe/message"

   // After
   import "github.com/fxsml/goengine/message"
   ```

2. **Update Message creation**
   ```go
   // Before (gopipe)
   msg := &message.Message{Data: data, Attributes: attrs}

   // After (goengine) - with validation
   msg, err := message.New(data, attrs)  // Validates CloudEvents
   ```

3. **Update Broker usage**
   ```go
   // Before (gopipe)
   sender := broker.NewHTTPSender(url)
   sender.Send(ctx, topic, msgs)

   // After (goengine)
   pub := adapters.NewHTTPPublisher(url)
   pub.Publish(ctx, msgs...)  // Topic from message attributes
   ```

### Phase 3: Deprecation in gopipe

1. Add deprecation notices to gopipe packages:
   ```go
   // Deprecated: Use github.com/fxsml/goengine/message instead.
   package message
   ```

2. Update gopipe CHANGELOG:
   ```markdown
   ## [Unreleased]

   ### Deprecated
   - `message/` package - Use goengine/message
   - `message/broker/` - Use goengine/adapters
   - `message/cqrs/` - Use goengine/cqrs
   ```

3. Keep deprecated packages for 2 minor versions

## User Migration Guide

### Simple Migration

For users just using message types:

```go
// 1. Update go.mod
require github.com/fxsml/goengine v0.1.0

// 2. Update imports
import "github.com/fxsml/goengine/message"

// 3. Update message creation (now validates)
msg, err := message.New(data, message.Attributes{
    message.AttrID:     "123",
    message.AttrSource: "/my/source",
    message.AttrType:   "com.example.event",
})
```

### Broker Migration

For users using brokers:

```go
// 1. Update imports
import "github.com/fxsml/goengine/adapters/nats"

// 2. Create subscriber (replaces Receiver)
sub := nats.NewSubscriber(conn, "topic")

// 3. Use in pipeline
for msg := range sub.Subscribe(ctx) {
    // Process message
}
```

## Timeline

| Phase | Description | Duration |
|-------|-------------|----------|
| 1 | Code migration | 1 week |
| 2 | API changes and testing | 2 weeks |
| 3 | Documentation | 1 week |
| 4 | Release goengine v0.1.0 | - |
| 5 | Deprecate gopipe packages | Next gopipe release |

## Acceptance Criteria

- [ ] All message code moved to goengine
- [ ] CloudEvents validation working
- [ ] All adapters functional
- [ ] CQRS handlers working
- [ ] Tests passing in goengine
- [ ] Migration guide complete
- [ ] gopipe deprecation notices added
- [ ] Example migration in examples/

## Related

- [PRO-0001-repo-init](../PRO-0001-repo-init/) - Repository setup
- [PRO-0003-message-standardization](../PRO-0003-message-standardization/) - Message changes
- [gopipe PRO-0030](../../../adr/PRO-0030-remove-sender-receiver.md) - Interface removal
