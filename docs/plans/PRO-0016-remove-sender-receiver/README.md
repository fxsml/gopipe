# PRO-0016: Remove Sender and Receiver Interfaces

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0030

## Overview

Remove the generic `Sender` and `Receiver` interfaces. Use `Subscriber[*Message]` for consuming and `channel.GroupBy` + broker clients for publishing.

## Goals

1. Deprecate `Sender` and `Receiver` interfaces
2. Remove `Publisher` and `Subscriber` wrappers
3. Move broker implementations to examples
4. Document migration path

## Task 1: Deprecation Phase

**Goal:** Mark interfaces as deprecated

```go
// Deprecated: Use broker-specific Subscriber[*Message] implementation.
// See ADR 0030 for migration guide.
type Sender interface { ... }

// Deprecated: Implement Subscriber[*Message] directly.
// See ADR 0030 for migration guide.
type Receiver interface { ... }
```

**Files to Modify:**
- `pubsub/sender.go` - Add deprecation notices
- `pubsub/receiver.go` - Add deprecation notices
- `pubsub/publisher.go` - Add deprecation notices
- `pubsub/subscriber.go` - Add deprecation notices

## Task 2: Move to Examples

**Goal:** Move broker implementations to examples/

```
examples/
├── http-broker/
│   ├── sender.go
│   └── receiver.go
└── io-broker/
    ├── sender.go
    └── receiver.go
```

**Files to Move:**
- `broker/http.go` → `examples/http-broker/`
- `broker/io.go` → `examples/io-broker/`

## Task 3: Removal Phase (Next Major Version)

**Goal:** Remove deprecated code

**Files to Delete:**
- `pubsub/sender.go`
- `pubsub/receiver.go`
- `pubsub/publisher.go`
- `pubsub/subscriber.go` (wrapper, not interface)
- `multiplex/` package

**Acceptance Criteria:**
- [ ] Deprecation notices added to all interfaces
- [ ] Broker implementations moved to examples
- [ ] Migration guide documented
- [ ] Tests updated
- [ ] CHANGELOG updated

## Migration Guide

**Consuming messages:**
```go
// Before
receiver := kafka.NewReceiver(config)
subscriber := message.NewSubscriber(receiver, subConfig)
msgs := subscriber.Subscribe(ctx, "orders")

// After
subscriber := kafka.NewSubscriber(config) // Implements Subscriber[*Message]
msgs := subscriber.Subscribe(ctx)
```

**Publishing messages:**
```go
// Before
sender := kafka.NewSender(config)
publisher := message.NewPublisher(sender, pubConfig)
publisher.Publish(ctx, msgs)

// After
groups := channel.GroupBy(msgs, topicFunc, config)
for group := range groups {
    kafkaProducer.Send(ctx, group.Key, group.Items)
}
```

## Related

- [PRO-0030](../../adr/PRO-0030-remove-sender-receiver.md) - ADR
- [PRO-0015](../PRO-0015-subscriber-patterns/) - Subscriber interface
