# Broker Adapters

This directory contains minimal broker adapters demonstrating the `Subscriber[*Message]` pattern from ADR 0030. These adapters bypass the deprecated `Sender`/`Receiver` interfaces, giving broker implementations full control over their connection models.

## Brokers

| Broker | Adapter | Key Concepts |
|--------|---------|--------------|
| **NATS** | `nats/` | Subjects, wildcards (`*`, `>`), queue groups |
| **Kafka** | `kafka/` | Topics, partitions, consumer groups, offsets |
| **RabbitMQ** | `rabbitmq/` | Exchanges, queues, bindings, routing keys |

## Running

```bash
# Start all brokers
docker-compose up -d

# Wait for brokers to be healthy
docker-compose ps

# Run specific demo
go run main.go nats
go run main.go kafka
go run main.go rabbitmq
go run main.go all
```

## Dependencies

Add to your `go.mod`:

```go
require (
    github.com/nats-io/nats.go v1.47.0
    github.com/segmentio/kafka-go v0.4.49
    github.com/rabbitmq/amqp091-go v1.10.0
)
```

## Why Not Sender/Receiver?

The deprecated `Sender`/`Receiver` interfaces force a simplistic model:

```go
// Old pattern - too restrictive
type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*Message, error)
}
```

Problems:
1. **Poll-based only**: Forces poll loop, doesn't fit push-based brokers
2. **Simple topic string**: Can't express Kafka partitions, RabbitMQ exchanges/bindings
3. **No backpressure**: Returns slice, can't use channel-based flow control
4. **No broker-specific features**: Wildcards, queue groups, consumer groups

## New Pattern

Each broker implements `Subscriber[*Message]` directly:

```go
// NATS - with subject wildcards
sub := nats.NewSubscriber(nats.SubscriberConfig{
    Subject: "orders.>",  // Wildcard subscription
    Queue:   "processors",
})
msgs := sub.Subscribe(ctx)

// Kafka - with consumer groups
sub := kafka.NewSubscriber(kafka.SubscriberConfig{
    Topics:        []string{"orders"},
    ConsumerGroup: "order-processor",
})
msgs := sub.Subscribe(ctx)

// RabbitMQ - with exchange/queue/binding
sub := rabbitmq.NewSubscriber(rabbitmq.SubscriberConfig{
    Exchange:     "orders",
    ExchangeType: "topic",
    Queue:        "order-queue",
    BindingKey:   "orders.#",
})
msgs := sub.Subscribe(ctx)
```

## Publishing with GroupBy

Use `channel.GroupBy` for batching by topic/routing key:

```go
// Extract destination from message attributes
topicFunc := func(msg *message.Message) string {
    return msg.Attributes["destination"].(string)
}

// Group messages by destination
groups := channel.GroupBy(processed, topicFunc, channel.GroupByConfig{
    MaxBatchSize: 100,
    MaxDuration:  time.Second,
})

// Publish batches
pub.PublishBatches(ctx, groups)
```

## Broker-Specific Attributes

Each adapter sets broker-specific attributes on messages:

### NATS
```go
msg.Attributes["nats.subject"]  // Original subject
msg.Attributes["nats.reply"]    // Reply-to subject
```

### Kafka
```go
msg.Attributes["kafka.topic"]     // Topic name
msg.Attributes["kafka.partition"] // Partition number
msg.Attributes["kafka.offset"]    // Message offset
msg.Attributes["kafka.key"]       // Partition key
```

### RabbitMQ
```go
msg.Attributes["rabbitmq.exchange"]     // Exchange name
msg.Attributes["rabbitmq.routing_key"]  // Routing key
msg.Attributes["rabbitmq.delivery_tag"] // Delivery tag for ack
msg.Attributes["rabbitmq.redelivered"]  // Was message redelivered?
```

## See Also

- [ADR 0030: Remove Sender and Receiver](../../docs/adr/0030-remove-sender-receiver.md)
- [ADR 0028: Subscriber Patterns](../../docs/adr/0028-generator-source-patterns.md)
- [Known Issues](../../docs/known-issues.md)
