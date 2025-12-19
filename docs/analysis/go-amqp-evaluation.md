# Azure go-amqp Evaluation for goengine

## Overview

[Azure/go-amqp](https://github.com/Azure/go-amqp) is a pure Go implementation of AMQP 1.0. This document evaluates its use in goengine as a universal AMQP 1.0 adapter, replacing service-specific SDKs.

## AMQP 1.0 vs AMQP 0.9.1

**Critical distinction:**

| Version | Brokers | Go Library |
|---------|---------|------------|
| AMQP 1.0 | Azure Service Bus, Azure Event Hubs, Apache Qpid, ActiveMQ Artemis | `Azure/go-amqp` |
| AMQP 0.9.1 | RabbitMQ (default), LavinMQ | `rabbitmq/amqp091-go` |

**RabbitMQ Note:** RabbitMQ uses AMQP 0.9.1 by default. AMQP 1.0 requires the [rabbitmq_amqp1_0 plugin](https://www.rabbitmq.com/docs/plugins#rabbitmq_amqp1_0), which has limitations.

## go-amqp API Overview

```go
import "github.com/Azure/go-amqp"

// 1. Create connection
conn, err := amqp.Dial(ctx, "amqps://host:5671", &amqp.ConnOptions{
    SASLType: amqp.SASLTypePlain("username", "password"),
})

// 2. Create session
session, err := conn.NewSession(ctx, nil)

// 3. Create sender
sender, err := session.NewSender(ctx, "queue-name", nil)

// 4. Create receiver
receiver, err := session.NewReceiver(ctx, "queue-name", nil)

// 5. Send message
err = sender.Send(ctx, &amqp.Message{
    Data: [][]byte{eventBytes},
    Properties: &amqp.MessageProperties{
        ContentType: "application/cloudevents+json",
    },
    ApplicationProperties: map[string]any{
        "ce_type":   "com.example.event",
        "ce_source": "/my-source",
    },
}, nil)

// 6. Receive message
msg, err := receiver.Receive(ctx, nil)
// Process...
receiver.AcceptMessage(ctx, msg)  // or RejectMessage, ReleaseMessage
```

## CloudEvents AMQP Binding Compliance

The [CloudEvents AMQP Protocol Binding](https://github.com/cloudevents/spec/blob/main/cloudevents/bindings/amqp-protocol-binding.md) defines how to map CloudEvents to AMQP messages.

### Binary Content Mode

CloudEvents attributes map to AMQP application properties with `ce_` prefix:

```go
msg := &amqp.Message{
    // Event data as-is
    Data: [][]byte{eventPayload},

    // CloudEvents attributes
    Properties: &amqp.MessageProperties{
        ContentType: "application/json",  // datacontenttype
        MessageID:   &eventID,            // id (optional mapping)
    },

    ApplicationProperties: map[string]any{
        "ce_specversion": "1.0",
        "ce_type":        "com.example.order.created",
        "ce_source":      "/orders/api",
        "ce_id":          "evt-12345",
        "ce_time":        "2025-01-15T10:30:00Z",
        // Extensions
        "ce_xdestination": "kafka://orders",
    },
}
```

### Structured Content Mode

Entire CloudEvent as JSON in message body:

```go
ceJSON, _ := json.Marshal(cloudEvent)
msg := &amqp.Message{
    Data: [][]byte{ceJSON},
    Properties: &amqp.MessageProperties{
        ContentType: "application/cloudevents+json",
    },
}
```

## Benefits of go-amqp Over Service-Specific SDKs

| Aspect | go-amqp | Azure SDK | AWS SDK |
|--------|---------|-----------|---------|
| Protocol | AMQP 1.0 standard | Proprietary wrapper | Proprietary |
| Portability | Any AMQP 1.0 broker | Azure only | AWS only |
| Dependencies | Minimal | Heavy | Heavy |
| CloudEvents | Manual (spec compliant) | Manual | Manual |
| Learning curve | One API | Per-service | Per-service |

### Supported Services with go-amqp

1. **Azure Service Bus** - Full support
2. **Azure Event Hubs** - Full support (AMQP endpoint)
3. **Apache Qpid** - Full support
4. **Apache ActiveMQ Artemis** - Full support
5. **Red Hat AMQ** - Full support
6. **RabbitMQ** - Limited (requires AMQP 1.0 plugin)

## goengine Integration Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        goengine                                  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                   AMQPAdapter                              │  │
│  │                                                            │  │
│  │  Uses: github.com/Azure/go-amqp                           │  │
│  │                                                            │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │  │
│  │  │ AMQPSender  │  │AMQPReceiver │  │  Connection │       │  │
│  │  │             │  │             │  │  Manager    │       │  │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘       │  │
│  │         │                │                │               │  │
│  │         └────────────────┼────────────────┘               │  │
│  │                          │                                │  │
│  │  CloudEvents ◄───────────┴───────────► CloudEvents        │  │
│  │  (Binary Mode)                         (Structured Mode)  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                   │
│                              ▼                                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │              AMQP 1.0 Broker (Any)                         │  │
│  │                                                            │  │
│  │  Azure Service Bus │ Event Hubs │ Qpid │ Artemis          │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Recommendation

### Use go-amqp for:

1. **Azure Service Bus** - Instead of Azure SDK
2. **Azure Event Hubs** - Instead of Azure SDK (via AMQP endpoint)
3. **Multi-cloud deployments** - Single codebase
4. **Apache Qpid/Artemis** - Native AMQP 1.0

### Do NOT use go-amqp for:

1. **RabbitMQ** - Use `rabbitmq/amqp091-go` (AMQP 0.9.1)
2. **Services requiring proprietary features** - May need native SDK

## Connection String Formats

```go
// Azure Service Bus
"amqps://<namespace>.servicebus.windows.net"

// Azure Event Hubs
"amqps://<namespace>.servicebus.windows.net/<eventhub>"

// Apache Qpid
"amqp://localhost:5672"

// With authentication
"amqps://username:password@host:5671"
```

## Error Handling and Reliability

```go
// Acknowledgment modes
receiver.AcceptMessage(ctx, msg)   // Success - remove from queue
receiver.RejectMessage(ctx, msg, nil)  // Permanent failure - dead letter
receiver.ReleaseMessage(ctx, msg)  // Temporary failure - requeue
receiver.ModifyMessage(ctx, msg, &amqp.ModifyMessageOptions{
    DeliveryFailed:    true,
    UndeliverableHere: true,
})

// Connection recovery
for {
    conn, err := amqp.Dial(ctx, url, opts)
    if err != nil {
        time.Sleep(backoff)
        continue
    }
    // Use connection...
    if connectionLost {
        conn.Close()
        continue
    }
}
```

## Sources

- [Azure/go-amqp GitHub](https://github.com/Azure/go-amqp)
- [CloudEvents AMQP Protocol Binding](https://github.com/cloudevents/spec/blob/main/cloudevents/bindings/amqp-protocol-binding.md)
- [Azure Service Bus AMQP Guide](https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-amqp-protocol-guide)
- [OASIS AMQP 1.0 Specification](https://www.amqp.org/specification/1.0/amqp-org-download)
