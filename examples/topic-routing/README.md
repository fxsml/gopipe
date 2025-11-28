# Topic Routing Example

This example demonstrates how to use `channel.GroupBy` to efficiently route and batch messages by topic for publishing to a message broker.

## Use Case

When publishing events to a message broker (like Kafka, RabbitMQ, NATS, etc.), it's often more efficient to publish messages in batches rather than one at a time. However, messages need to be grouped by their destination topic before batching.

The `GroupBy` function solves this by:
1. Grouping incoming events by their topic
2. Batching events for each topic separately
3. Emitting batches when they reach a size threshold OR a time threshold

## How It Works

```go
batches := channel.GroupBy(
    events,
    func(msg *message.Message[Event]) string {
        return msg.Payload().Topic  // Extract topic as grouping key
    },
    5,                    // Max 5 events per batch
    200*time.Millisecond, // Or flush after 200ms
)

for batch := range batches {
    publisher.PublishBatch(batch.Key, batch.Items)
}
```

## Key Features

- **Dynamic Grouping**: Events are grouped by topic on-the-fly
- **Hybrid Batching**: Batches are emitted based on size OR time
- **Message Acknowledgment**: Proper ack/nack handling for reliable delivery
- **Backpressure**: Channel-based flow control prevents overwhelming the publisher

## Output

The example produces output showing:
- Real-time batch publishing to different topics
- Statistics on throughput, batch sizes, and acknowledgments
- Demonstration of both size-based and time-based batch emission

## Applications

This pattern is useful for:
- Event-driven architectures
- Multi-tenant systems (grouping by tenant ID)
- Priority queuing (grouping by priority level)
- Partition routing (grouping by partition key)
- Any scenario requiring efficient grouped batch processing
