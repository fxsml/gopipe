# ADR 0011: CloudEvents Compatibility

**Date:** 2025-12-08
**Status:** Superseded by ADR 0018

> **Historical Note:** This ADR proposed a separate CloudEvent type. The actual implementation
> integrated CloudEvents attributes directly into `message.Attributes`. See ADR 0018.

## Context

CloudEvents is a CNCF specification for describing event data in a common way. It's widely adopted in cloud-native ecosystems. gopipe's Message doesn't natively support CloudEvents attributes (source, specversion, type).

## Decision

Implement CloudEvents compatibility through a `cloudevents` subpackage:

```go
// CloudEvent extends Message with CloudEvents required attributes
type CloudEvent struct {
    *message.Message
    Source      string // Required: event origin URI
    SpecVersion string // Required: "1.0"
    Type        string // Required: event type
}

// Converters
func FromMessage(msg *message.Message) (*CloudEvent, error)
func ToMessage(ce *CloudEvent) *message.Message

// Marshaler for CloudEvents JSON format
type CloudEventsMarshaler struct{}
```

Mapping: gopipe AttrID -> id, AttrSubject -> subject, PropContentType -> datacontenttype, PropCreatedAt -> time, Data -> data.

## Consequences

**Positive:**
- Interoperability with cloud-native systems
- Non-breaking: optional package
- Bidirectional conversion

**Negative:**
- Additional dependency on CloudEvents spec
- Source/SpecVersion/Type must be provided for outbound events

## Links

- CloudEvents Specification: https://cloudevents.io/
- ADR 0010: Message Package Structure
- Package: `message/cloudevents`
