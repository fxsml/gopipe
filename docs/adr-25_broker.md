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
4. **IO implementation**: Stream-based broker using `io.Reader`/`io.Writer` with JSONL format
5. **HTTP implementation**: Webhook-style broker for HTTP POST send/receive
6. **Configuration**: `CloseTimeout`, `SendTimeout`, `ReceiveTimeout`, `BufferSize`

Topic design:
- String-based with "/" separator (e.g., "orders/created")
- Topics created on-the-fly (no pre-registration)
- Pattern matching utilities for wildcards (`+`, `#`)

Constructors:
- `NewBroker[T](Config)` - in-memory broker
- `NewSender[T](Broker)` - sender-only view
- `NewReceiver[T](Broker)` - receiver-only view
- `NewIOBroker[T](io.Reader, io.Writer, IOConfig)` - stream-based broker
- `NewIOSender[T](io.Writer, IOConfig)` - stream sender
- `NewIOReceiver[T](io.Reader, IOConfig)` - stream receiver
- `NewHTTPSender[T](url, HTTPConfig)` - webhook sender
- `NewHTTPReceiver[T](HTTPConfig, bufferSize)` - HTTP POST receiver

Wire format (IO broker):
- JSON Lines (JSONL) format
- Envelope: `{topic, properties, payload, timestamp}`
- Pluggable marshaler (defaults to JSON)

Wire format (HTTP broker):
- `X-Gopipe-Topic` header for topic
- `X-Gopipe-Prop-*` headers for message properties
- Request body contains payload with `Content-Type` header
- Returns 201 Created on success

### Consequences

**Positive**
- Consistent abstraction for all message transport scenarios
- In-memory broker enables unit testing without external dependencies
- IO broker enables file persistence, logging, and IPC via pipes
- HTTP broker enables webhook integrations and REST-style messaging
- Hierarchical topics support domain-driven message organization
- Channel-based receive integrates naturally with gopipe pipelines
- Common test suite validates all implementations uniformly

**Negative**
- Generic type parameter requires explicit type at broker creation
- In-memory broker lacks persistence and durability guarantees
- IO broker requires manual lifecycle management of underlying streams
- HTTP receiver requires external HTTP server setup
- Pattern matching (wildcards) not integrated into core broker (utility only)

**Identified Patterns for Future Work**
- `channel.FanOut` - broadcast to multiple channels with backpressure
- `channel.Subscribe` - topic pattern matching with channel output
- Persistent broker implementations (embedded DB)
