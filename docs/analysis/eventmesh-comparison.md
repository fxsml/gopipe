# Apache EventMesh vs goengine Comparison

## What is Apache EventMesh?

[Apache EventMesh](https://github.com/apache/eventmesh) is a **serverless event middleware** platform that provides:

- A network of interconnected event brokers (event mesh)
- Multi-protocol support (HTTP, TCP, gRPC, MQTT)
- Plugin-based connector architecture for various event stores
- Serverless workflow orchestration (CNCF Serverless Workflow spec)
- Multi-cloud and hybrid deployment support

**Key point:** EventMesh is **infrastructure middleware** that runs as a separate service/sidecar.

## Architecture Comparison

### EventMesh Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Apache EventMesh                             │
│                                                                 │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐   │
│  │   Producer   │────►│ EventMesh    │────►│   Consumer   │   │
│  │     SDK      │     │   Runtime    │     │     SDK      │   │
│  └──────────────┘     └──────┬───────┘     └──────────────┘   │
│                              │                                  │
│                    ┌─────────▼─────────┐                       │
│                    │   Connector API   │                       │
│                    └─────────┬─────────┘                       │
│                              │                                  │
│  ┌───────────┬───────────┬──┴──────────┬───────────┐          │
│  │ RocketMQ  │  Kafka    │   Pulsar    │   Redis   │          │
│  │ Connector │ Connector │  Connector  │ Connector │          │
│  └───────────┴───────────┴─────────────┴───────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

### goengine Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                 Go Microservice Process                         │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                      goengine                             │  │
│  │                                                           │  │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐  │  │
│  │  │Subscriber│──►│ Router  │──►│ Handler │──►│Publisher│  │  │
│  │  │ (NATS)  │   │         │   │         │   │ (NATS)  │  │  │
│  │  └─────────┘   └─────────┘   └─────────┘   └─────────┘  │  │
│  │                                                           │  │
│  │  Direct broker connections - no middleware layer          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                  │
│                    ┌─────────▼─────────┐                       │
│                    │  Broker (Direct)  │                       │
│                    └───────────────────┘                       │
└─────────────────────────────────────────────────────────────────┘
```

## Feature Comparison

| Feature | EventMesh | goengine |
|---------|-----------|----------|
| **Deployment** | Separate service/sidecar | Embedded library |
| **Language** | Java (SDK: Java, Go, Python, Rust) | Go only |
| **Protocol Bridge** | Yes (HTTP↔MQTT↔gRPC↔TCP) | No (direct broker protocols) |
| **Event Store** | Plugin-based (RocketMQ, Kafka, etc.) | Direct adapter implementation |
| **Workflow Orchestration** | CNCF Serverless Workflow DSL | Handler/Pipe composition |
| **Multi-Cloud** | Native mesh networking | Application-level (manual) |
| **CloudEvents** | First-class support | First-class support |
| **Type Safety** | SDK-based | Compile-time generics |
| **Complexity** | High (infrastructure) | Low (library) |
| **Operational Overhead** | Separate deployment | None (in-process) |

## Key Differences

### 1. Deployment Model

**EventMesh:** Runs as infrastructure middleware
- Separate process (runtime or sidecar)
- Requires operational management
- Protocol translation at network level
- Multi-service orchestration

**goengine:** Embedded library
- In-process, zero network hop for internal routing
- No operational overhead
- Direct broker connections
- Single-service focus

### 2. Use Case Focus

**EventMesh:**
- Enterprise event infrastructure
- Multi-protocol environments (IoT + Web + Backend)
- Cross-cloud event routing
- Organization-wide event mesh
- Protocol translation/bridging

**goengine:**
- Single Go microservice event handling
- Type-safe event processing
- Internal event loops (sagas, CQRS)
- Minimal latency requirements
- Go-native development experience

### 3. Complexity Trade-offs

**EventMesh:**
```
+ Multi-protocol support
+ Cross-service orchestration
+ Centralized event management
- Operational complexity
- Additional network hop
- Java ecosystem dependency
```

**goengine:**
```
+ Zero infrastructure overhead
+ Type-safe handlers
+ Nanosecond internal loops
+ Native Go experience
- Go-only
- No protocol bridging
- Per-service implementation
```

## Can EventMesh Replace goengine?

**Short answer: No - they serve different purposes.**

### EventMesh provides:
1. **Network-level event mesh** - routing events between services
2. **Protocol bridging** - HTTP client talks to MQTT device
3. **Centralized workflow** - orchestrate multiple services
4. **Event store abstraction** - switch backends without code changes

### goengine provides:
1. **In-process event handling** - zero-copy internal routing
2. **Type-safe handlers** - compile-time guarantees
3. **Go-native CQRS/Saga** - patterns as code
4. **Minimal latency** - no serialization for internal loops

### Complementary Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Event Mesh Network                              │
│                                                                         │
│  ┌─────────────────────┐                    ┌─────────────────────┐    │
│  │   Order Service     │                    │  Payment Service    │    │
│  │  ┌───────────────┐  │   EventMesh       │  ┌───────────────┐  │    │
│  │  │   goengine    │  │◄──────────────────►│  │   goengine    │  │    │
│  │  │               │  │    (cross-svc)     │  │               │  │    │
│  │  │ ┌───────────┐ │  │                    │  │ ┌───────────┐ │  │    │
│  │  │ │  Handler  │ │  │                    │  │ │  Handler  │ │  │    │
│  │  │ │  (typed)  │ │  │                    │  │ │  (typed)  │ │  │    │
│  │  │ └───────────┘ │  │                    │  │ └───────────┘ │  │    │
│  │  │ ┌───────────┐ │  │                    │  │ ┌───────────┐ │  │    │
│  │  │ │ Internal  │ │  │                    │  │ │ Internal  │ │  │    │
│  │  │ │   Loop    │ │  │                    │  │ │   Loop    │ │  │    │
│  │  │ └───────────┘ │  │                    │  │ └───────────┘ │  │    │
│  │  └───────────────┘  │                    │  └───────────────┘  │    │
│  └─────────────────────┘                    └─────────────────────┘    │
│                                                                         │
│  EventMesh handles: Cross-service routing, protocol bridging           │
│  goengine handles:  In-service processing, type safety, internal loops │
└─────────────────────────────────────────────────────────────────────────┘
```

## When to Use Each

### Use EventMesh when:
- Building organization-wide event infrastructure
- Need protocol translation (IoT devices, legacy systems)
- Multi-language polyglot environment
- Cross-cloud event routing requirements
- Centralized workflow orchestration

### Use goengine when:
- Building individual Go microservices
- Need type-safe event handlers
- Implementing CQRS/Saga patterns
- Require minimal latency for internal events
- Want embedded library (no operational overhead)

### Use Both when:
- Go microservices need internal event processing (goengine)
- Services communicate via enterprise event mesh (EventMesh)
- goengine publishes to/subscribes from EventMesh

## goengine Vision

goengine's vision is to provide an **infrastructure layer for Go microservices** to be fully event-driven:

1. **Embedded Library** - Not infrastructure, but in-process
2. **Type-Safe** - Compile-time guarantees via generics
3. **Zero-Copy Internal Loops** - No serialization for internal routing
4. **CloudEvents Native** - First-class CloudEvents support
5. **Convention over Configuration** - Sensible defaults, full customization
6. **Minimal Dependencies** - Core has no external deps, adapters optional

This is fundamentally different from EventMesh which aims to be **enterprise event infrastructure**.

## Sources

- [Apache EventMesh GitHub](https://github.com/apache/eventmesh)
- [Apache EventMesh Homepage](https://eventmesh.apache.org/)
- [EventMesh Serverless Platform - InfoQ](https://www.infoq.com/news/2023/04/eventmesh-serverless/)
- [What is an Event Mesh? - Red Hat](https://www.redhat.com/en/topics/integration/what-is-an-event-mesh)
- [Event Broker vs Event Mesh - Solace](https://solace.com/what-is-an-event-broker/)
