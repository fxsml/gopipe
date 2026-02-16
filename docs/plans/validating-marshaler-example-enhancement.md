# Plan: Validating Marshaler Example Enhancement

**Status:** Complete
**Related ADRs:** [0022](../adr/0022-message-package-redesign.md), [0024](../adr/0024-http-cloudevents-adapter.md), [0025](../adr/0025-json-schema-validation.md)

## Overview

Enhance the JSON Schema validation example to demonstrate real-world HTTP CloudEvents patterns with proper command/event separation and schema serving capabilities.

## Context

The branch `claude/explore-message-validation-BnHzL` introduces JSON Schema validation via `message/jsonschema/marshaler.go`. The current example (07-validating-marshaler) demonstrates validation but lacks real-world patterns like HTTP ingestion, schema discovery endpoints, and proper CQRS boundaries.

CloudEvents best practices recommend:
- Serving schemas at HTTP endpoints (referenced by `dataschema` attribute)
- Command/event separation (not domain models)
- Schema versioning via URI changes
- Schema discovery for consumers/brokers

## Current Implementation Analysis

### Library Implementation (`message/jsonschema/marshaler.go`)
- ✅ Single schema per Go type via `map[reflect.Type]*entry`
- ✅ Uses JSON Schema Draft 2020-12 with `santhosh-tekuri/jsonschema/v6`
- ✅ `Schema()` method returns `json.RawMessage` for serving
- ✅ Comprehensive test coverage in `marshaler_test.go`
- ✅ Validation hooks into engine unmarshal automatically

### Example Limitations
- ❌ No HTTP server (hardcoded in-memory test messages)
- ❌ Uses domain model `CreateOrder` instead of command/event separation
- ❌ No schema serving endpoints
- ❌ No demonstration of channel/pipe primitives
- ❌ Missing real-world validation failure scenarios

## Goals

1. Demonstrate HTTP CloudEvents server accepting validated commands
2. Show command → event transformation with separate types
3. Serve schemas at HTTP endpoints following CloudEvents `dataschema` pattern
4. Use gopipe channel/pipe primitives for message flow
5. Keep example minimal but production-ready

## CloudEvents Best Practices (Research Summary)

### Schema Serving Pattern
- **AWS EventBridge**: Schema registry with URI references
- **Google Cloud Eventarc**: `dataschema` points to HTTP JSON Schema endpoints
- **CloudEvents Spec**: Incompatible schema versions should use different URIs

### Key Attributes
- `dataschema` (optional): URI pointing to the schema for `data` field
- `datacontenttype`: Media type (e.g., `application/json`)
- Schema URIs should be HTTP endpoints returning JSON Schema documents

### Versioning Strategy
- Different schema versions = different URIs
- No mandated versioning pattern in spec
- Recommended: Separate Go types per version (e.g., `OrderCreatedV1`, `OrderCreatedV2`)

**Sources:**
- [CloudEvents Specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md)
- [CloudEvents JSON Format](https://github.com/cloudevents/spec/blob/main/cloudevents/formats/json-format.md)
- [Azure Event Grid CloudEvents](https://learn.microsoft.com/en-us/azure/event-grid/cloud-event-schema)
- [Google Cloud Eventarc](https://cloud.google.com/eventarc/docs/cloudevents-json)
- [AWS EventBridge CloudEvents](https://aws.amazon.com/blogs/compute/sending-and-receiving-cloudevents-with-amazon-eventbridge/)

## Tasks

### Task 1: Define Command and Event Types

**Goal:** Demonstrate CQRS pattern with separate input/output types

**Implementation:**
```go
// CreateOrderCommand - input command (HTTP boundary)
type CreateOrderCommand struct {
    OrderID string  `json:"order_id"`
    Amount  float64 `json:"amount"`
}

const createOrderCommandSchema = `{
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "properties": {
        "order_id": { "type": "string", "minLength": 1 },
        "amount":   { "type": "number", "exclusiveMinimum": 0 }
    },
    "required": ["order_id", "amount"],
    "additionalProperties": false
}`

// OrderCreatedEvent - output event (downstream consumers)
type OrderCreatedEvent struct {
    OrderID   string    `json:"order_id"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
}

const orderCreatedEventSchema = `{...}`
```

**Files to Modify:**
- `examples/07-validating-marshaler/main.go` - Replace domain model with command/event

**Acceptance Criteria:**
- [ ] Command type represents user intent (input boundary)
- [ ] Event type represents fact (output boundary)
- [ ] Both types have strict JSON Schema definitions
- [ ] Schemas registered at startup

### Task 2: HTTP Server with CloudEvents Ingestion

**Goal:** Accept CloudEvents via HTTP (binary or structured mode)

**Implementation:**
```go
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    // Setup marshaler with schemas
    marshaler := jsonschema.NewMarshaler()
    marshaler.MustRegister(CreateOrderCommand{}, createOrderCommandSchema)
    marshaler.MustRegister(OrderCreatedEvent{}, orderCreatedEventSchema)

    // Setup engine
    engine := message.NewEngine(message.EngineConfig{
        Marshaler: marshaler,
        ErrorHandler: func(msg *message.Message, err error) {
            log.Printf("❌ Validation failed: %v", err)
        },
    })

    // HTTP subscriber for commands
    subscriber := cehttp.NewSubscriber(cehttp.SubscriberConfig{BufferSize: 100})
    inputCh, _ := subscriber.Subscribe(ctx)
    engine.AddRawInput("http-commands", nil, inputCh)

    // Command handler: CreateOrderCommand → OrderCreatedEvent
    engine.AddHandler("process-order", nil, message.NewCommandHandler(
        func(ctx context.Context, cmd CreateOrderCommand) ([]OrderCreatedEvent, error) {
            log.Printf("✅ Processing order: %s ($%.2f)", cmd.OrderID, cmd.Amount)
            return []OrderCreatedEvent{{
                OrderID:   cmd.OrderID,
                Status:    "confirmed",
                CreatedAt: time.Now(),
            }}, nil
        },
        message.CommandHandlerConfig{
            Source: "/order-processor",
            Naming: message.KebabNaming,
        },
    ))

    // Start engine
    engineDone, _ := engine.Start(ctx)

    // HTTP server
    mux := http.NewServeMux()
    mux.Handle("POST /events", subscriber)
    // ... schema endpoints (Task 3)
}
```

**Files to Modify:**
- `examples/07-validating-marshaler/main.go` - Add HTTP server with CloudEvents subscriber

**Acceptance Criteria:**
- [ ] HTTP server listens on `:8080`
- [ ] Accepts CloudEvents in binary mode (headers: `Ce-*`)
- [ ] Accepts CloudEvents in structured mode (`Content-Type: application/cloudevents+json`)
- [ ] Valid commands processed successfully
- [ ] Invalid commands rejected with clear error messages

### Task 3: Schema Serving Endpoints

**Goal:** Serve JSON schemas at HTTP endpoints for discovery

**Implementation:**
```go
// Schema catalog endpoint
mux.HandleFunc("GET /schemas", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    catalog := map[string]json.RawMessage{
        "create-order-command": marshaler.Schema(CreateOrderCommand{}),
        "order-created-event":  marshaler.Schema(OrderCreatedEvent{}),
    }
    json.NewEncoder(w).Encode(catalog)
})

// Individual schema endpoint
mux.HandleFunc("GET /schema/{type}", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/schema+json")

    var schema json.RawMessage
    switch r.PathValue("type") {
    case "create-order-command":
        schema = marshaler.Schema(CreateOrderCommand{})
    case "order-created-event":
        schema = marshaler.Schema(OrderCreatedEvent{})
    default:
        http.NotFound(w, r)
        return
    }

    if schema == nil {
        http.NotFound(w, r)
        return
    }

    w.Write(schema)
})
```

**Files to Modify:**
- `examples/07-validating-marshaler/main.go` - Add schema endpoints

**Acceptance Criteria:**
- [ ] `GET /schemas` returns catalog of all schemas
- [ ] `GET /schema/{type}` returns individual JSON Schema
- [ ] Returns 404 for unknown schema types
- [ ] Content-Type headers set correctly

### Task 4: Channel/Pipe Primitives for stdout

**Goal:** Demonstrate gopipe channel/pipe usage for event printing

**Implementation:**
```go
// Output events to stdout using pipe
outputCh, _ := engine.AddRawOutput("events", nil)

printer := pipe.NewSinkPipe(func(ctx context.Context, raw *message.RawMessage) error {
    var event OrderCreatedEvent
    json.Unmarshal(raw.Data, &event)
    log.Printf("📤 Event: %s | order=%s status=%s",
        raw.Type(), event.OrderID, event.Status)
    return nil
}, pipe.Config{Concurrency: 1})

printer.Pipe(ctx, outputCh)
```

**Files to Modify:**
- `examples/07-validating-marshaler/main.go` - Add pipe-based stdout printer

**Acceptance Criteria:**
- [ ] Uses `pipe.NewSinkPipe` for stdout printing
- [ ] Events printed in readable format
- [ ] Demonstrates channel-based flow separation

### Task 5: Validation Demonstration

**Goal:** Show clear validation success/failure scenarios

**Implementation:**
Include curl examples in startup message and comments:

```bash
# Valid request
curl -X POST http://localhost:8080/events \
  -H "Ce-Specversion: 1.0" -H "Ce-Id: 1" \
  -H "Ce-Type: create-order-command" -H "Ce-Source: /client" \
  -d '{"order_id":"ORD-001","amount":100}'

# Invalid: missing required field
curl -X POST http://localhost:8080/events \
  -H "Ce-Specversion: 1.0" -H "Ce-Id: 2" \
  -H "Ce-Type: create-order-command" -H "Ce-Source: /client" \
  -d '{"order_id":"ORD-002"}'

# Invalid: wrong type
curl -X POST http://localhost:8080/events \
  -H "Ce-Specversion: 1.0" -H "Ce-Id: 3" \
  -H "Ce-Type: create-order-command" -H "Ce-Source: /client" \
  -d '{"order_id":"ORD-003","amount":"free"}'

# Invalid: empty string (minLength violation)
curl -X POST http://localhost:8080/events \
  -H "Ce-Specversion: 1.0" -H "Ce-Id: 4" \
  -H "Ce-Type: create-order-command" -H "Ce-Source: /client" \
  -d '{"order_id":"","amount":50}'
```

**Files to Modify:**
- `examples/07-validating-marshaler/main.go` - Add usage instructions and comments

**Acceptance Criteria:**
- [ ] Example includes curl commands in comments/output
- [ ] Valid request shows successful processing
- [ ] Invalid requests show clear error messages
- [ ] Error handler logs validation failures with context

## Implementation Order

```
Task 1 (Command/Event Types)
    ↓
Task 2 (HTTP Server)
    ↓
Task 3 (Schema Endpoints) ← Task 4 (Pipe stdout)
    ↓
Task 5 (Documentation)
```

Tasks 3 and 4 can be implemented in parallel after Task 2.

## Files to Create/Modify

| File | Changes |
|------|---------|
| `examples/07-validating-marshaler/main.go` | Complete rewrite with HTTP server, schema endpoints, pipes |

**No changes to library code** - the existing `jsonschema.Marshaler` already supports all required functionality.

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Example runs with `go run ./examples/07-validating-marshaler`
- [ ] HTTP server accepts valid CloudEvents and processes them
- [ ] Invalid CloudEvents rejected with clear validation errors
- [ ] Schema catalog endpoint returns all schemas
- [ ] Individual schema endpoints return JSON Schema documents
- [ ] Uses pipe primitives for stdout printing
- [ ] Demonstrates command/event separation pattern
- [ ] Example is minimal (~150 lines) but production-ready
- [ ] Comments include curl commands for testing

## Verification Steps

1. **Run example**: `go run ./examples/07-validating-marshaler`
2. **Valid request**: Should process and emit OrderCreatedEvent
3. **Invalid requests**: Should log validation errors (3 test cases)
4. **Schema catalog**: `curl http://localhost:8080/schemas` returns JSON
5. **Individual schema**: `curl http://localhost:8080/schema/create-order-command` returns JSON Schema
6. **Structured mode**: Test with full CloudEvents JSON payload
7. **Shutdown**: Ctrl+C should gracefully shutdown server and engine
