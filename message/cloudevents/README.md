# CloudEvents Integration

This package integrates gopipe's [message] package with [CloudEvents SDK](https://github.com/cloudevents/sdk-go) protocol bindings.

## Features

- Wrap any CloudEvents `protocol.Receiver` as a gopipe input source
- Wrap any CloudEvents `protocol.Sender` as a gopipe output sink
- Bridge CloudEvents `Finish()` acknowledgment to gopipe's `Acking` callbacks

## Quick Start

```go
import (
    "github.com/fxsml/gopipe/message"
    cloudevents "github.com/fxsml/gopipe/message/cloudevents"
    cejetstream "github.com/cloudevents/sdk-go/protocol/nats_jetstream/v2"
)

// Create CloudEvents protocol instances
receiver, _ := cejetstream.NewConsumerFromConn(conn, stream, subject, jsOpts, subOpts)
sender, _ := cejetstream.NewSenderFromConn(conn, stream, subject, jsOpts)

// Create adapters
sub := cloudevents.NewSubscriber(receiver, cloudevents.SubscriberConfig{})
pub := cloudevents.NewPublisher(sender, cloudevents.PublisherConfig{})

// Wire raw input through Router via unmarshal/marshal pipes
router := message.NewRouter(message.PipeConfig{})
router.AddHandler("orders", nil, handler)

rawIn, _ := sub.Subscribe(ctx)
unmarshal := message.NewUnmarshalPipe(router, message.NewJSONMarshaler(), message.PipeConfig{})
typedIn, _ := unmarshal.Pipe(ctx, rawIn)
typedOut, _ := router.Pipe(ctx, typedIn)
marshal := message.NewMarshalPipe(message.NewJSONMarshaler(), message.PipeConfig{})
rawOut, _ := marshal.Pipe(ctx, typedOut)

pub.Publish(ctx, rawOut)

// Start the external protocol loop (SDK-specific)
go receiver.OpenInbound(ctx)
```

[message]: https://pkg.go.dev/github.com/fxsml/gopipe/message

## API

```go
// Create adapters
sub := cloudevents.NewSubscriber(receiver, cloudevents.SubscriberConfig{
    Buffer:      100,  // output channel buffer size
    Concurrency: 1,    // receive goroutines
})
pub := cloudevents.NewPublisher(sender, cloudevents.PublisherConfig{
    Concurrency: 1,    // send goroutines
})

// Produces/consumes raw channels directly
inCh, _ := sub.Subscribe(ctx)
pub.Publish(ctx, outCh)
```

### Conversion Functions

For manual conversion between formats:

```go
// CloudEvents Event -> Message (raw []byte Data)
raw, err := cloudevents.FromCloudEvent(event, acking)

// Message (raw []byte Data) -> CloudEvents Event
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
