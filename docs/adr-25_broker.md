## ADR-25: Interface Broker for Message Pub/Sub

### Context

Gopipe provides message processing capabilities but lacks a standardized abstraction for message transport. Users integrating with message brokers (Kafka, RabbitMQ, NATS) implement ad-hoc solutions. A unified interface would enable:
- Consistent API across different broker implementations
- Easy testing with in-memory broker
- Integration with gopipe's channel utilities

### Decision

Introduce a `message/broker` package with:

1. **Interfaces**: `Sender[T]` and `Receiver[T]` for decoupled send/receive operations
2. **Broker interface**: Combines Sender and Receiver with lifecycle management
3. **In-memory implementation**: Reference broker for testing and simple use cases
4. **Configuration**: `CloseTimeout`, `SendTimeout`, `ReceiveTimeout`, `BufferSize`

Topic design:
- String-based with "/" separator (e.g., "orders/created")
- Topics created on-the-fly (no pre-registration)
- Pattern matching utilities for wildcards (`+`, `#`)

Three constructors:
- `NewBroker[T](Config)` - full broker with send/receive
- `NewSender[T](Broker)` - sender-only view of broker
- `NewReceiver[T](Broker)` - receiver-only view of broker

### Consequences

**Positive**
- Consistent abstraction for all message transport scenarios
- In-memory broker enables unit testing without external dependencies
- Hierarchical topics support domain-driven message organization
- Channel-based receive integrates naturally with gopipe pipelines

**Negative**
- Generic type parameter requires explicit type at broker creation
- In-memory broker lacks persistence and durability guarantees
- Pattern matching (wildcards) not integrated into core broker (utility only)

**Identified Patterns for Future Work**
- `channel.FanOut` - broadcast to multiple channels with backpressure
- `channel.Subscribe` - topic pattern matching with channel output
- Persistent broker implementations (file-based, embedded DB)
