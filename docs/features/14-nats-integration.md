# Feature: NATS Integration

**Package:** `gopipe-nats` (external package)
**Status:** Proposed (Future External Package)
**Related ADRs:**
- [ADR 0023](../adr/0023-internal-message-loop.md) - Internal Message Loop
- [ADR 0024](../adr/0024-destination-attribute.md) - Destination Attribute

## Summary

NATS integration for gopipe, providing advanced internal messaging capabilities while keeping the core gopipe package dependency-free. NATS can be embedded as a Go dependency, enabling full messaging features (persistence, clustering, JetStream) for internal routing.

## Motivation

- **Zero external infrastructure**: NATS can run embedded in Go process
- **Advanced features**: Persistence, clustering, wildcards, JetStream
- **Same interface**: Implements MessageChannel for drop-in replacement
- **External package**: Keeps core gopipe lightweight and dependency-free

## Package Structure

```
github.com/fxsml/gopipe-nats/
├── channel.go       # NATSChannel implementing MessageChannel
├── sender.go        # NATSSender for external NATS routing
├── receiver.go      # NATSReceiver for external NATS input
├── embedded.go      # Embedded NATS server option
├── config.go        # Configuration
├── jetstream.go     # JetStream support (optional)
└── examples/
    ├── embedded/    # Embedded NATS internal loop
    └── external/    # External NATS cluster
```

## Implementation

### NATSChannel (MessageChannel Implementation)

```go
package gopipe_nats

import (
    "context"
    "github.com/fxsml/gopipe/message"
    "github.com/nats-io/nats.go"
)

// NATSChannel implements MessageChannel using NATS
type NATSChannel struct {
    conn       *nats.Conn
    serializer *message.Serializer
    config     NATSChannelConfig
}

type NATSChannelConfig struct {
    URL           string            // NATS URL (or "" for embedded)
    Embedded      bool              // Run embedded NATS server
    EmbeddedPort  int               // Port for embedded server
    Serializer    *message.Serializer
    SubjectPrefix string            // Prefix for internal subjects
}

func NewNATSChannel(config NATSChannelConfig) (*NATSChannel, error) {
    var conn *nats.Conn
    var err error

    if config.Embedded {
        // Start embedded NATS server
        conn, err = startEmbeddedNATS(config.EmbeddedPort)
    } else {
        conn, err = nats.Connect(config.URL)
    }
    if err != nil {
        return nil, err
    }

    return &NATSChannel{
        conn:       conn,
        serializer: config.Serializer,
        config:     config,
    }, nil
}

func (c *NATSChannel) Publish(ctx context.Context, msg *message.Message) error {
    dest, err := msg.ParsedDestination()
    if err != nil {
        return err
    }

    // Convert path to NATS subject
    subject := c.pathToSubject(dest.Path)

    // Serialize message
    data, err := c.serializeMessage(msg)
    if err != nil {
        return err
    }

    return c.conn.Publish(subject, data)
}

func (c *NATSChannel) Subscribe(ctx context.Context, topics ...string) (<-chan *message.Message, error) {
    out := make(chan *message.Message, 100)

    for _, topic := range topics {
        subject := c.pathToSubject(topic)

        _, err := c.conn.Subscribe(subject, func(m *nats.Msg) {
            msg, err := c.deserializeMessage(m.Data)
            if err != nil {
                // Log error
                return
            }
            out <- msg
        })
        if err != nil {
            close(out)
            return nil, err
        }
    }

    // Handle context cancellation
    go func() {
        <-ctx.Done()
        close(out)
    }()

    return out, nil
}

func (c *NATSChannel) Close() error {
    c.conn.Close()
    return nil
}

// pathToSubject converts gopipe path to NATS subject
// gopipe://orders/commands -> gopipe.orders.commands
func (c *NATSChannel) pathToSubject(path string) string {
    subject := strings.ReplaceAll(path, "/", ".")
    if c.config.SubjectPrefix != "" {
        return c.config.SubjectPrefix + "." + subject
    }
    return "gopipe." + subject
}
```

### Embedded NATS Server

```go
package gopipe_nats

import (
    "github.com/nats-io/nats-server/v2/server"
    "github.com/nats-io/nats.go"
)

// startEmbeddedNATS starts an in-process NATS server
func startEmbeddedNATS(port int) (*nats.Conn, error) {
    opts := &server.Options{
        Port:      port,
        NoLog:     true,
        NoSigs:    true,
        JetStream: true,  // Enable JetStream for persistence
    }

    ns, err := server.NewServer(opts)
    if err != nil {
        return nil, err
    }

    go ns.Start()
    if !ns.ReadyForConnections(10 * time.Second) {
        return nil, errors.New("nats server not ready")
    }

    // Connect to embedded server
    return nats.Connect(ns.ClientURL())
}
```

### NATSSender (External NATS Routing)

```go
package gopipe_nats

// NATSSender sends messages to external NATS
type NATSSender struct {
    conn       *nats.Conn
    serializer *message.Serializer
}

func NewNATSSender(url string, serializer *message.Serializer) (*NATSSender, error) {
    conn, err := nats.Connect(url)
    if err != nil {
        return nil, err
    }
    return &NATSSender{conn: conn, serializer: serializer}, nil
}

func (s *NATSSender) Send(ctx context.Context, msgs []*message.Message) error {
    for _, msg := range msgs {
        dest, err := msg.ParsedDestination()
        if err != nil {
            return err
        }

        // nats://subject or nats://cluster/subject
        subject := dest.Path
        if dest.Host != "" {
            // Could use host for cluster routing
        }

        data, err := s.serializeMessage(msg)
        if err != nil {
            return err
        }

        if err := s.conn.Publish(subject, data); err != nil {
            return err
        }
    }
    return nil
}
```

### NATSReceiver (External NATS Input)

```go
package gopipe_nats

// NATSReceiver receives messages from external NATS
type NATSReceiver struct {
    conn       *nats.Conn
    subjects   []string
    serializer *message.Serializer
}

func NewNATSReceiver(url string, subjects []string, serializer *message.Serializer) (*NATSReceiver, error) {
    conn, err := nats.Connect(url)
    if err != nil {
        return nil, err
    }
    return &NATSReceiver{
        conn:       conn,
        subjects:   subjects,
        serializer: serializer,
    }, nil
}

func (r *NATSReceiver) Receive(ctx context.Context) (<-chan *message.Message, error) {
    out := make(chan *message.Message, 100)

    for _, subject := range r.subjects {
        _, err := r.conn.Subscribe(subject, func(m *nats.Msg) {
            msg, err := r.deserializeMessage(m.Data)
            if err != nil {
                return
            }
            out <- msg
        })
        if err != nil {
            close(out)
            return nil, err
        }
    }

    go func() {
        <-ctx.Done()
        close(out)
    }()

    return out, nil
}
```

## Usage Examples

### Embedded NATS for Internal Loop

```go
import (
    "github.com/fxsml/gopipe/message"
    gopipe_nats "github.com/fxsml/gopipe-nats"
)

// Create NATS channel (embedded)
natsChannel, err := gopipe_nats.NewNATSChannel(gopipe_nats.NATSChannelConfig{
    Embedded:     true,
    EmbeddedPort: 4222,
    Serializer:   serializer,
})
if err != nil {
    log.Fatal(err)
}
defer natsChannel.Close()

// Use with InternalLoop
loop := message.NewInternalLoop(
    message.WithChannel(natsChannel),  // Swap NoopChannel for NATS
)

loop.Route("orders", handleOrders)
loop.Route("shipping", handleShipping)

loop.Start(ctx)
```

### External NATS Cluster

```go
// Connect to external NATS cluster
natsChannel, err := gopipe_nats.NewNATSChannel(gopipe_nats.NATSChannelConfig{
    URL:           "nats://nats-1:4222,nats://nats-2:4222",
    SubjectPrefix: "myapp",
    Serializer:    serializer,
})

// Use for internal messaging across instances
loop := message.NewInternalLoop(
    message.WithChannel(natsChannel),
)
```

### Mixed Internal/External

```go
loop := message.NewInternalLoop()

// Register NATS sender for external routing
natsSender, _ := gopipe_nats.NewNATSSender("nats://external:4222", serializer)
loop.RegisterExternal("nats", natsSender)

// Handler can route internally or externally
loop.Route("complete", func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    return []*message.Message{
        // Internal (uses embedded/NoopChannel)
        message.MustNew(AuditEvent{}, message.Attributes{
            message.AttrDestination: "gopipe://audit",
            // ...
        }),
        // External NATS
        message.MustNew(NotificationEvent{}, message.Attributes{
            message.AttrDestination: "nats://notifications.order-complete",
            // ...
        }),
    }, nil
})
```

## NATS vs NoopChannel Comparison

| Feature | NoopChannel | NATSChannel |
|---------|-------------|-------------|
| Dependencies | None | nats.go |
| Persistence | No | Yes (JetStream) |
| Clustering | No | Yes |
| Wildcards | No | Yes (*, >) |
| Cross-process | No | Yes |
| Performance | Fastest | Very fast |
| Use case | Simple pipelines | Production systems |

## JetStream Support (Optional)

```go
// Enable JetStream for persistence and exactly-once delivery
natsChannel, _ := gopipe_nats.NewNATSChannel(gopipe_nats.NATSChannelConfig{
    URL: "nats://localhost:4222",
    JetStream: &gopipe_nats.JetStreamConfig{
        Stream:   "GOPIPE",
        Subjects: []string{"gopipe.>"},
        Storage:  nats.FileStorage,
        Replicas: 3,
    },
})
```

## Flow with NATS

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                      gopipe InternalLoop with NATSChannel                        │
│                                                                                  │
│    ┌─────────────────────────────────────────────────────────────────────────┐  │
│    │                    NATSChannel (Embedded or External)                    │  │
│    │                                                                          │  │
│    │   gopipe.orders   gopipe.shipping   gopipe.audit   gopipe.complete      │  │
│    │        │                │                │               │              │  │
│    └────────┼────────────────┼────────────────┼───────────────┼──────────────┘  │
│             │                │                │               │                 │
│             ▼                ▼                ▼               ▼                 │
│    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│    │OrderHandler │  │ShipHandler  │  │AuditHandler │  │CompleteHdlr │          │
│    └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │
│           │                │                │                │                  │
│           ▼                ▼                ▼                │                  │
│     gopipe://         gopipe://        gopipe://             │                  │
│     shipping          audit            (sink)               │                  │
│                                                              │                  │
└──────────────────────────────────────────────────────────────┼──────────────────┘
                                                               │
                                                               │ nats://external/...
                                                               ▼
                                                    ┌────────────────────┐
                                                    │ External NATS      │
                                                    │ (different cluster)│
                                                    └────────────────────┘
```

## Files (External Package)

```
gopipe-nats/
├── go.mod
├── go.sum
├── channel.go           # NATSChannel
├── channel_test.go
├── sender.go            # NATSSender
├── receiver.go          # NATSReceiver
├── embedded.go          # Embedded NATS server
├── jetstream.go         # JetStream support
├── config.go            # Configuration types
├── serialization.go     # Message serialization
└── examples/
    ├── embedded/main.go
    └── external/main.go
```

## Dependencies

```go
// go.mod for gopipe-nats
module github.com/fxsml/gopipe-nats

go 1.21

require (
    github.com/fxsml/gopipe v0.11.0
    github.com/nats-io/nats.go v1.31.0
    github.com/nats-io/nats-server/v2 v2.10.0  // For embedded
)
```

## Related Features

- [13-internal-message-loop](13-internal-message-loop.md) - Core loop implementation
- [11-contenttype-serialization](11-contenttype-serialization.md) - Message serialization
