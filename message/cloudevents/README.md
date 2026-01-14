# CloudEvents Integration

This package integrates gopipe's message engine with [CloudEvents SDK](https://github.com/cloudevents/sdk-go) protocol bindings.

## Features

- Wrap any CloudEvents `protocol.Receiver` as a gopipe input source
- Wrap any CloudEvents `protocol.Sender` as a gopipe output sink
- Bridge CloudEvents `Finish()` acknowledgment to gopipe's `Acking` callbacks
- Plugin API for simplified engine registration

## Quick Start

```go
import (
    cloudevents "github.com/fxsml/gopipe/message/cloudevents"
    cejetstream "github.com/cloudevents/sdk-go/protocol/nats_jetstream/v2"
)

// Create CloudEvents protocol instances
receiver, _ := cejetstream.NewConsumerFromConn(conn, stream, subject, jsOpts, subOpts)
sender, _ := cejetstream.NewSenderFromConn(conn, stream, subject, jsOpts)

// Register with engine using plugins
engine.AddPlugin(
    cloudevents.SubscriberPlugin(
        ctx, "input", nil,
        receiver, cloudevents.SubscriberConfig{},
    ),
    cloudevents.PublisherPlugin(
        ctx, "output", nil,
        sender, cloudevents.PublisherConfig{},
    ),
)

// Start the external protocol loop (SDK-specific)
go receiver.OpenInbound(ctx)

// Start engine
engine.Start(ctx)
```

## API

### Plugins (Recommended)

```go
// SubscriberPlugin wraps a CloudEvents Receiver as an engine input
cloudevents.SubscriberPlugin(
    ctx context.Context,
    name string,
    matcher message.Matcher,
    receiver protocol.Receiver,
    cfg SubscriberConfig,
) message.Plugin

// PublisherPlugin wraps a CloudEvents Sender as an engine output
cloudevents.PublisherPlugin(
    ctx context.Context,
    name string,
    matcher message.Matcher,
    sender protocol.Sender,
    cfg PublisherConfig,
) message.Plugin
```

### Direct Usage

For more control over the wiring:

```go
// Create adapters
sub := cloudevents.NewSubscriber(receiver, cloudevents.SubscriberConfig{
    Buffer:      100,  // output channel buffer size
    Concurrency: 1,    // receive goroutines
})
pub := cloudevents.NewPublisher(sender, cloudevents.PublisherConfig{
    Concurrency: 1,    // send goroutines
})

// Wire to engine
inCh, _ := sub.Subscribe(ctx)
engine.AddRawInput("input", nil, inCh)

outCh, _ := engine.AddRawOutput("output", nil)
pub.Publish(ctx, outCh)
```

### Conversion Functions

For manual conversion between formats:

```go
// CloudEvents Event -> RawMessage
raw, err := cloudevents.FromCloudEvent(event, acking)

// RawMessage -> CloudEvents Event
event, err := cloudevents.ToCloudEvent(raw)
```

## Acknowledgment Bridge

| gopipe | CloudEvents | Broker Effect |
|--------|-------------|---------------|
| `msg.Ack()` | `ceMsg.Finish(nil)` | ACK - message processed |
| `msg.Nack(err)` | `ceMsg.Finish(err)` | NACK - redelivery |

## Supported Protocols

Any CloudEvents SDK protocol binding works:

- HTTP
- Kafka (Sarama, Confluent)
- AMQP
- NATS / NATS JetStream
- Google PubSub
- MQTT

See [CloudEvents SDK Protocol Implementations](https://cloudevents.github.io/sdk-go/protocol_implementations.html).

## Example

See [examples/06-cloudevents-nats](../../examples/06-cloudevents-nats) for a complete NATS JetStream example.
