# Feature: CloudEvents Protocol Support

**Package:** `message/cloudevents`
**Status:** ✅ Implemented
**Related ADRs:**
- [ADR 0011](../adr/0011-cloudevents-compatibility.md) - CloudEvents Compatibility (Superseded)
- [ADR 0018](../adr/0018-cloudevents-terminology.md) - CloudEvents Terminology (Accepted)

## Summary

Implements CloudEvents v1.0.2 HTTP Protocol Binding for interoperability with CloudEvents-compatible systems. Supports both binary and structured content modes, plus JSONL format for IO operations.

## CloudEvents HTTP Binding

Implements the [CloudEvents HTTP Protocol Binding v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/http-protocol-binding.md).

### Content Modes

**Binary Mode**: CloudEvents attributes in HTTP headers, data in body
```http
POST /webhook HTTP/1.1
ce-specversion: 1.0
ce-type: com.example.order.created
ce-source: /orders
ce-id: 12345
ce-subject: orders.12345
Content-Type: application/json

{"orderId": "12345", "amount": 100}
```

**Structured Mode**: CloudEvents envelope with data embedded
```http
POST /webhook HTTP/1.1
Content-Type: application/cloudevents+json

{
  "specversion": "1.0",
  "type": "com.example.order.created",
  "source": "/orders",
  "id": "12345",
  "subject": "orders.12345",
  "datacontenttype": "application/json",
  "data": {"orderId": "12345", "amount": 100}
}
```

**Batched Mode**: Array of CloudEvents in structured format
```http
POST /webhook HTTP/1.1
Content-Type: application/cloudevents-batch+json

[
  {"specversion": "1.0", "type": "...", "source": "...", "id": "1", ...},
  {"specversion": "1.0", "type": "...", "source": "...", "id": "2", ...}
]
```

## CloudEvents Package API

### Encoding/Decoding
```go
// Encode message to CloudEvent
func ToCloudEvent(msg *message.Message) *CloudEvent

// Decode CloudEvent to message
func FromCloudEvent(ce *CloudEvent) *message.Message

// CloudEvent type
type CloudEvent struct {
    SpecVersion     string         `json:"specversion"`
    Type            string         `json:"type"`
    Source          string         `json:"source"`
    ID              string         `json:"id"`
    Time            string         `json:"time,omitempty"`
    Subject         string         `json:"subject,omitempty"`
    DataContentType string         `json:"datacontenttype,omitempty"`
    Data            json.RawMessage `json:"data,omitempty"`
    Extensions      map[string]any  `json:"-"`
}
```

### HTTP Headers
```go
const (
    HeaderPrefix      = "ce-"
    HeaderSpecVersion = "ce-specversion"
    HeaderType        = "ce-type"
    HeaderSource      = "ce-source"
    HeaderID          = "ce-id"
    HeaderTime        = "ce-time"
    HeaderSubject     = "ce-subject"
    // ... etc
)
```

### JSON Format (for IO Broker)
```go
// Marshal to JSON (one CloudEvent)
func MarshalJSON(msg *message.Message) ([]byte, error)

// Unmarshal from JSON
func UnmarshalJSON(data []byte) (*message.Message, error)
```

## HTTP Broker Integration

The HTTP broker uses CloudEvents format:

```go
// Sending (binary mode)
sender := broker.NewHTTPSender(httpClient, broker.HTTPSenderConfig{
    DefaultURL: "https://api.example.com/webhook",
    ContentMode: broker.ContentModeBinary,
})

// Receiving
receiver := broker.NewHTTPReceiver()
handler := receiver.Handler() // Returns http.Handler
http.Handle("/webhook", handler)
```

## IO Broker Integration

The IO broker uses JSONL (one CloudEvent per line):

```go
broker := broker.NewIOBroker(os.Stdout, os.Stdin)

// Output format (JSONL)
{"specversion":"1.0","type":"order.created","source":"/orders","id":"123","data":{...}}
{"specversion":"1.0","type":"payment.completed","source":"/payments","id":"456","data":{...}}
```

## Attribute Mapping

CloudEvents attributes map to `message.Attributes`:

| CloudEvent Field | Message Attribute | Description |
|-----------------|-------------------|-------------|
| `id` | `message.AttrID` | Unique identifier |
| `source` | `message.AttrSource` | Event source |
| `specversion` | `message.AttrSpecVersion` | CloudEvents version |
| `type` | `message.AttrType` | Event type |
| `subject` | `message.AttrSubject` | Subject/routing key |
| `time` | `message.AttrTime` | Timestamp (RFC3339) |
| `datacontenttype` | `message.AttrDataContentType` | Content type |
| Extensions | Custom attributes | Additional metadata |

## Files Changed

- `message/cloudevents/cloudevents.go` - CloudEvents core implementation
- `message/cloudevents/cloudevents_test.go` - Tests including HTTP binding
- `message/broker/http.go` - HTTP broker with CloudEvents support
- `message/broker/http_cloudevents_test.go` - CloudEvents HTTP tests
- `message/broker/io.go` - IO broker with JSONL CloudEvents
- `message/attributes.go` - CloudEvents attribute constants

## Usage Example

```go
// Manual CloudEvents conversion
msg := &message.Message{
    Data: []byte(`{"orderId": "123"}`),
    Attributes: message.Attributes{
        message.AttrID:      "evt-123",
        message.AttrType:    "order.created",
        message.AttrSource:  "/orders",
        message.AttrSubject: "orders.123",
    },
}

// Convert to CloudEvent
ce := cloudevents.ToCloudEvent(msg)
jsonData, _ := json.Marshal(ce)

// Convert back
msg2 := cloudevents.FromCloudEvent(ce)
```

## Related Features

- [03-message-pubsub](03-message-pubsub.md) - HTTP and IO brokers use CloudEvents
- [02-message-core-refactor](02-message-core-refactor.md) - CloudEvents-aligned attributes
