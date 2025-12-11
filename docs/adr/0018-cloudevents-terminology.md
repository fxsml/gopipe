# ADR 0018: CloudEvents Terminology Alignment

**Date:** 2025-01-10
**Status:** Accepted
**Supersedes:** ADR 0011

## Context

CloudEvents specification v1.0.2 defines standard terminology for event data:
- **attributes** (not "properties") - metadata that describes the event
- **data** (not "payload") - the event payload
- Required attributes: `id`, `source`, `specversion`, `type`
- Optional attributes: `datacontenttype`, `subject`, `time`

gopipe originally used different terminology:
- `Properties` instead of `Attributes`
- `Payload` instead of `Data`
- `Prop*` prefix instead of `Attr*` prefix

This created friction when integrating with CloudEvents-based systems and made the codebase inconsistent with industry standards.

## Decision

Align gopipe's terminology with CloudEvents spec v1.0.2:

### Type Renames
| Old | New |
|-----|-----|
| `message.Properties` | `message.Attributes` |
| `TypedMessage.Payload` | `TypedMessage.Data` |
| `PropertyProvider` | `AttributeProvider` |

### Constant Renames (message package)
| Old | New | CloudEvents Key |
|-----|-----|-----------------|
| `PropID` | `AttrID` | `id` |
| `PropSource` | `AttrSource` | `source` |
| `PropSpecVersion` | `AttrSpecVersion` | `specversion` |
| `PropType` | `AttrType` | `type` |
| `PropSubject` | `AttrSubject` | `subject` |
| `PropTime` | `AttrTime` | `time` |
| `PropDataContentType` | `AttrDataContentType` | `datacontenttype` |
| `PropCorrelationID` | `AttrCorrelationID` | `correlationid` |
| `PropDeadline` | `AttrDeadline` | `deadline` |
| `PropTopic` | `AttrTopic` | `topic` |

### Function Renames (cqrs package)
| Old | New |
|-----|-----|
| `CombineProps()` | `CombineAttrs()` |
| `WithProperty()` | `WithAttribute()` |

### Method Renames
| Old | New |
|-----|-----|
| `Attributes.CreatedAt()` | `Attributes.EventTime()` |
| `Attributes.ContentType()` | `Attributes.DataContentType()` |

### Wire Format (message/broker/io.go)
The `Envelope` struct for JSON Lines format updated:
```go
type Envelope struct {
    Topic      string          `json:"topic"`
    Attributes map[string]any  `json:"attributes,omitempty"`
    Data       json.RawMessage `json:"data"`
    Timestamp  time.Time       `json:"time"`
}
```

### gopipe Extensions
These attributes are gopipe-specific extensions not in CloudEvents core:
- `correlationid` - distributed tracing correlation
- `deadline` - message processing timeout
- `topic` - pub/sub routing (routing-only, not forwarded to brokers)

## Rationale

1. **Industry Standard**: CloudEvents is the CNCF standard for event description
2. **Interoperability**: Easier integration with CloudEvents-based systems
3. **Consistency**: Developers familiar with CloudEvents immediately understand gopipe's message model
4. **Future-Proofing**: Enables CloudEvents JSON format output for HTTP adapters
5. **Protobuf Compatibility**: Using standard terminology prepares for future protobuf serialization support

## Consequences

### Positive
- gopipe messages align with CloudEvents terminology
- Easier documentation and onboarding for developers
- Clear path to CloudEvents JSON format HTTP adapters
- Standard naming enables future protobuf schemas

### Negative
- Breaking change for existing code (requires migration)
- Wire format change in io.go Envelope (requires data migration for existing JSONL files)

### Migration Guide
```go
// Old code
msg := message.New(payload, message.Properties{
    message.PropType:    "OrderCreated",
    message.PropSubject: "order/123",
})
data := msg.Payload

// New code
msg := message.New(data, message.Attributes{
    message.AttrType:    "OrderCreated",
    message.AttrSubject: "order/123",
})
data := msg.Data
```

## Links

- CloudEvents Specification: https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md
- CloudEvents JSON Format: https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/formats/json-format.md
- ADR 0011: CloudEvents Compatibility (superseded)
