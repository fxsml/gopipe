# CloudEvents SDK Protocol Bindings Evaluation

## Overview

The [CloudEvents SDK for Go](https://github.com/cloudevents/sdk-go) provides native protocol bindings that can potentially be used directly in goengine. This document evaluates each binding for compatibility with our architecture.

## Available Protocol Bindings

| Protocol | Package | Underlying Library | Status |
|----------|---------|-------------------|--------|
| HTTP | `protocol/http` | `net/http` | ✅ Stable |
| NATS | `protocol/nats/v2` | `nats-io/nats.go` | ✅ Stable |
| NATS JetStream | `protocol/nats_jetstream/v2` | `nats-io/nats.go` | ✅ Stable (v3) |
| Kafka (Sarama) | `protocol/kafka_sarama/v2` | `IBM/sarama` | ✅ Stable |
| Kafka (Confluent) | `protocol/kafka_confluent/v2` | `confluentinc/confluent-kafka-go` | ✅ New |
| AMQP | `protocol/amqp/v2` | `pack.ag/amqp` | ⚠️ Legacy |
| MQTT | `protocol/mqtt_paho/v2` | `eclipse/paho.golang` | ✅ Stable |
| PubSub | `protocol/pubsub/v2` | `cloud.google.com/go/pubsub` | ✅ Stable |
| Go Channels | `protocol/gochan/v2` | Native | ✅ Testing |

## SDK Protocol Interface

```go
// Core interfaces from protocol package
type Sender interface {
    Send(ctx context.Context, m binding.Message, transformers ...binding.Transformer) error
}

type Receiver interface {
    Receive(ctx context.Context) (binding.Message, error)
}

type Opener interface {
    OpenInbound(ctx context.Context) error
}

type Closer interface {
    Close(ctx context.Context) error
}
```

## Direct Usage Analysis

### Option A: Use SDK Protocol Bindings Directly

```go
// Using SDK NATS binding directly
import nats_ce "github.com/cloudevents/sdk-go/protocol/nats/v2"

protocol, _ := nats_ce.NewProtocol(natsURL, sendSubject, recvSubject, nil)
client, _ := cloudevents.NewClient(protocol)

// Send
client.Send(ctx, event)

// Receive
client.StartReceiver(ctx, func(evt cloudevents.Event) {
    // Process event
})
```

**Pros:**
- CloudEvents compliance guaranteed
- Less code to maintain
- Protocol-specific optimizations built-in

**Cons:**
- SDK patterns may not fit goengine architecture
- Limited control over connection management
- Receiver callback pattern differs from channel-based Subscriber

### Option B: Use SDK for Serialization Only (Recommended)

```go
// Direct NATS connection, SDK for event encoding
import "github.com/nats-io/nats.go"
import cloudevents "github.com/cloudevents/sdk-go/v2"

// Subscribe with native NATS
sub, _ := nc.Subscribe(subject, func(msg *nats.Msg) {
    // Parse with SDK
    event := cloudevents.NewEvent()
    json.Unmarshal(msg.Data, &event)

    // Convert to internal Message
    internal := marshaller.FromEvent(event)
    msgChan <- internal
})
```

**Pros:**
- Full control over connection lifecycle
- Channel-based Subscriber fits goengine pattern
- Can implement custom acknowledgment strategies
- Consistent pattern across all brokers

**Cons:**
- Must handle CloudEvents wire format ourselves
- More code per adapter

## Per-Protocol Evaluation

### HTTP - Direct SDK Usage Viable

The HTTP binding works well for webhook-style integrations.

```go
import ce_http "github.com/cloudevents/sdk-go/protocol/http"

// Server mode - receive webhooks
protocol, _ := ce_http.New(ce_http.WithPort(8080))
client, _ := cloudevents.NewClient(protocol)

go client.StartReceiver(ctx, func(evt cloudevents.Event) {
    // Forward to internal channel
})

// Client mode - send webhooks
protocol, _ := ce_http.New(ce_http.WithTarget("https://example.com/events"))
client, _ := cloudevents.NewClient(protocol)
client.Send(ctx, event)
```

**Recommendation:** ✅ **Use SDK directly** - HTTP is stateless, callback pattern works

### NATS - Hybrid Approach

```go
import nats_ce "github.com/cloudevents/sdk-go/protocol/nats/v2"

// Option 1: SDK Protocol
consumer, _ := nats_ce.NewConsumer(natsURL, subject, nil,
    nats_ce.WithQueueSubscriber("goengine-group"))
// Problem: Returns binding.Message, not cloudevents.Event directly

// Option 2: Direct NATS + SDK Marshalling (Recommended)
nc, _ := nats.Connect(natsURL)
sub, _ := nc.QueueSubscribe(subject, "goengine-group", func(msg *nats.Msg) {
    event := cloudevents.NewEvent()
    json.Unmarshal(msg.Data, &event)
    // Convert and forward
})
```

**Recommendation:** ⚠️ **Hybrid** - Use nats.go directly, SDK for marshalling

### NATS JetStream - Hybrid Approach

JetStream requires explicit acknowledgment - our Subscriber pattern fits well.

```go
import "github.com/nats-io/nats.go"

js, _ := nc.JetStream()
sub, _ := js.PullSubscribe(subject, "goengine-consumer")

// Pull-based consumption fits Subscriber pattern
msgs, _ := sub.Fetch(10)
for _, msg := range msgs {
    event := parseCloudEvent(msg.Data)
    internal := marshaller.FromEvent(event)

    if err := process(internal); err != nil {
        msg.Nak()
    } else {
        msg.Ack()
    }
}
```

**Recommendation:** ⚠️ **Hybrid** - Direct JetStream API, SDK for marshalling

### Kafka - Hybrid Approach

Consumer groups and partition assignment need direct control.

```go
import "github.com/IBM/sarama"

// Consumer group handler
type handler struct {
    msgChan chan *Message
}

func (h *handler) ConsumeClaim(session sarama.ConsumerGroupSession,
    claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        event := parseCloudEvent(msg.Value)
        internal := marshaller.FromEvent(event)
        h.msgChan <- internal
        session.MarkMessage(msg, "")
    }
    return nil
}
```

**Recommendation:** ⚠️ **Hybrid** - Direct Sarama, SDK for marshalling

### AMQP 1.0 - See go-amqp Evaluation Below

**Recommendation:** ⚠️ **Hybrid** - Use go-amqp directly, SDK for marshalling

## Summary: Recommended Approach

| Protocol | Connection | Marshalling | Notes |
|----------|------------|-------------|-------|
| HTTP | SDK | SDK | Stateless, callback OK |
| NATS | nats.go | SDK | Queue groups, direct control |
| JetStream | nats.go | SDK | Ack/Nak control needed |
| Kafka | sarama | SDK | Consumer groups |
| AMQP 1.0 | go-amqp | SDK | See below |
| MQTT | paho | SDK | QoS control |
| RabbitMQ | amqp091-go | Manual | AMQP 0.9.1, no SDK |

## Sources

- [CloudEvents SDK-Go Protocol Package](https://pkg.go.dev/github.com/cloudevents/sdk-go/v2/protocol)
- [NATS Protocol Binding](https://pkg.go.dev/github.com/cloudevents/sdk-go/protocol/nats/v2)
- [Kafka Sarama Protocol Binding](https://pkg.go.dev/github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2)
- [AMQP Protocol Binding](https://pkg.go.dev/github.com/cloudevents/sdk-go/protocol/amqp/v2)
