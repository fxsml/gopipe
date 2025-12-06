# ADR 0003: Remove Noisy Properties

## Status
Accepted

## Context
Message had many property constants and accessors that added noise without clear use cases:
- `PropDeliveryCount` / `DeliveryCount()`
- `PropReplyTo` / `ReplyTo()`
- `PropSequenceNumber` / `SequenceNumber()`
- `PropPartitionKey` / `PartitionKey()`
- `PropPartitionOffset` / `PartitionOffset()`
- `PropTTL` / `TTL()`

These can be added by users as custom properties if needed.

## Decision
Remove the above properties and their accessors.

Keep only essential properties:
- `PropID` - unique identifier
- `PropCorrelationID` - request tracing
- `PropCreatedAt` - timestamp
- `PropDeadline` - processing deadline
- `PropSubject` - topic/subject
- `PropContentType` - payload format

## Consequences

**Breaking Changes:**
- Removed 6 property constants and accessors
- Users needing these must add as custom properties

**Benefits:**
- Simpler, focused API
- Less cognitive overhead
- Easier to understand core functionality

**Drawbacks:**
- Users need slightly more code for removed properties
- Migration effort for existing code using these properties
