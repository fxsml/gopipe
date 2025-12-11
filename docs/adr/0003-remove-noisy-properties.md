# ADR 0003: Remove Noisy Attributes

**Date:** 2025-11-01
**Status:** Implemented

> **Historical Note:** This ADR uses mixed terminology (`Prop*` and `Attr*`).
> The current API uses `Attr*` constants consistently (e.g., `AttrTime`, `AttrDeadline`).

## Context
Message had many property constants and accessors that added noise without clear use cases:
- `PropDeliveryCount` / `DeliveryCount()`
- `PropReplyTo` / `ReplyTo()`
- `PropSequenceNumber` / `SequenceNumber()`
- `PropPartitionKey` / `PartitionKey()`
- `PropPartitionOffset` / `PartitionOffset()`
- `PropTTL` / `TTL()`

These can be added by users as custom attributes if needed.

## Decision
Remove the above attributes and their accessors.

Keep only essential attributes:
- `AttrID` - unique identifier
- `AttrCorrelationID` - request tracing
- `PropCreatedAt` - timestamp
- `PropDeadline` - processing deadline
- `AttrSubject` - topic/subject
- `PropContentType` - data format

## Consequences

**Breaking Changes:**
- Removed 6 property constants and accessors
- Users needing these must add as custom attributes

**Benefits:**
- Simpler, focused API
- Less cognitive overhead
- Easier to understand core functionality

**Drawbacks:**
- Users need slightly more code for removed attributes
- Migration effort for existing code using these attributes
