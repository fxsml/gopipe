# Validation vs. Marshaling Separation - Design Evolution

**Status:** Resolved — Option A implemented (Pure Separation via Registry + Middleware)
**Related Plan:** validating-marshaler-example-enhancement.md
**Related ADR:** [0027](../adr/0027-json-schema-validation.md)

## Context

Current `message/jsonschema` implementation couples validation with Go type marshaling:

```go
// Must define Go type
type CreateOrder struct { OrderID string }

// Register schema WITH Go type
marshaler.MustRegister(CreateOrder{}, schema)

// Validation happens during marshal/unmarshal
marshaler.Unmarshal(data, &order)  // validates + deserializes
```

**Problem:** Proxy use case needs validation WITHOUT Go types:

```
HTTP → validate → AMQP
  ↓                 ↓
RawMessage     RawMessage

No handler, no Go types - just validate bytes and forward.
```

## Options Considered

### Option A: Validation Middleware (Pure Separation)

Extract validation into standalone middleware:

```go
// Registry: CloudEvents type → schema (no Go types)
type Registry struct {
    schemas map[string]*compiledSchema  // eventType → schema
}

func (r *Registry) Register(eventType string, schemaJSON string)
func (r *Registry) Validate(eventType string, data []byte) error
func (r *Registry) Schema(eventType string) json.RawMessage
func (r *Registry) Schemas() json.RawMessage

// Middleware for RawMessage validation
func NewValidationMiddleware(registry *Registry) middleware.Middleware[*RawMessage, *RawMessage] {
    return func(next ProcessFunc) ProcessFunc {
        return func(ctx context.Context, msg *RawMessage) (*RawMessage, error) {
            if err := registry.Validate(msg.Type(), msg.Data); err != nil {
                return nil, fmt.Errorf("validation: %w", err)
            }
            return next(ctx, msg)
        }
    }
}

// Usage: Proxy
subscriber := http.NewSubscriber(...)
rawInput, _ := subscriber.Subscribe(ctx)

// Add validation middleware
validator := jsonschema.NewRegistry()
validator.Register("order.created", orderSchema)
validator.Register("order.cancelled", cancelSchema)

validationMW := jsonschema.NewValidationMiddleware(validator)

// Pipe: input → validate → output (no types!)
pipe := pipe.NewPassthroughPipe(pipe.Config{})
pipe.Use(validationMW)
validated, _ := pipe.Pipe(ctx, rawInput)

publisher := amqp.NewPublisher(...)
publisher.Publish(ctx, validated)

// Usage: With types (for handlers)
marshaler := jsonschema.NewMarshaler(jsonschema.Config{
    Registry: validator,  // Share registry!
    Naming:   message.DotNaming,
})
marshaler.RegisterType(CreateOrder{})  // Links Go type to "order.created"
```

**Pros:**
- ✅ Pure separation: validation ≠ marshaling
- ✅ Supports proxy use case (no Go types needed)
- ✅ Middleware is idiomatic gopipe pattern
- ✅ Registry is reusable (middleware + marshaler share schemas)
- ✅ Composable - add validation at any pipeline stage
- ✅ Can validate raw bytes before deciding to unmarshal

**Cons:**
- ❌ More components (Registry, Marshaler, Middleware)
- ❌ Schema registration requires explicit eventType string (no type derivation)
- ❌ Less type-safe when used without Go types
- ❌ Breaking change to existing API

### Option B: Keep Current Design (Status Quo)

Keep validation inside Marshaler, proxy must define dummy types:

```go
// Proxy workaround
type OrderCreated struct{}  // Empty type just for validation
type OrderCancelled struct{}

marshaler := jsonschema.NewMarshaler(jsonschema.Config{})
marshaler.MustRegister(OrderCreated{}, orderSchema)
marshaler.MustRegister(OrderCancelled{}, cancelSchema)

// Validate by round-trip
var dummy OrderCreated
if err := marshaler.Unmarshal(msg.Data, &dummy); err != nil {
    return fmt.Errorf("validation: %w", err)
}
// Ignore dummy, forward msg as-is
```

**Pros:**
- ✅ No breaking changes
- ✅ Single component (Marshaler)
- ✅ Type-safe when used with real handlers
- ✅ Automatic type derivation via naming

**Cons:**
- ❌ Requires dummy types for proxy use case
- ❌ Wasteful unmarshaling just for validation
- ❌ Validation tightly coupled to serialization
- ❌ Can't validate without defining Go types
- ❌ Not idiomatic for proxy pattern

### Option C: Hybrid (Recommended)

Provide both Registry (core) and Marshaler (typed wrapper):

```go
// Core: Registry (CloudEvents type → schema)
type Registry struct {
    mu       sync.RWMutex
    compiler *jschema.Compiler
    schemas  map[string]*entry  // eventType → schema
}

func NewRegistry() *Registry
func (r *Registry) Register(eventType string, schemaJSON string) error
func (r *Registry) MustRegister(eventType string, schemaJSON string)
func (r *Registry) Validate(eventType string, data []byte) error
func (r *Registry) Schema(eventType string) json.RawMessage
func (r *Registry) Schemas() json.RawMessage  // Composed catalog

// Middleware: Validates RawMessage using Registry
func NewValidationMiddleware(registry *Registry) middleware.Middleware[*RawMessage, *RawMessage]

// Marshaler: Adds Go type mapping on top of Registry
type Marshaler struct {
    registry *Registry
    types    map[string]reflect.Type     // eventType → Go type
    reverse  map[reflect.Type]string     // Go type → eventType
    naming   message.EventTypeNaming
}

func NewMarshaler(cfg Config) *Marshaler {
    return &Marshaler{
        registry: cfg.Registry,  // Share or create new
        types:    make(map[string]reflect.Type),
        reverse:  make(map[reflect.Type]string),
        naming:   cfg.Naming,
    }
}

// Register Go type (derives eventType via naming)
func (m *Marshaler) Register(v any, schemaJSON string) error {
    t := elemType(v)
    eventType := m.naming.EventType(t)

    // Register schema in Registry
    if err := m.registry.Register(eventType, schemaJSON); err != nil {
        return err
    }

    // Map Go type ↔ eventType
    m.types[eventType] = t
    m.reverse[t] = eventType
    return nil
}

// Unmarshal: validates via Registry, then deserializes
func (m *Marshaler) Unmarshal(data []byte, v any) error {
    t := elemType(v)
    eventType, ok := m.reverse[t]
    if !ok {
        return json.Unmarshal(data, v)  // No schema, pass through
    }

    // Validate using Registry
    if err := m.registry.Validate(eventType, data); err != nil {
        return fmt.Errorf("validation: %w", err)
    }

    return json.Unmarshal(data, v)
}

// Marshal: serializes, then validates via Registry
func (m *Marshaler) Marshal(v any) ([]byte, error) {
    data, err := json.Marshal(v)
    if err != nil {
        return nil, err
    }

    t := elemType(v)
    eventType, ok := m.reverse[t]
    if !ok {
        return data, nil  // No schema, pass through
    }

    if err := m.registry.Validate(eventType, data); err != nil {
        return nil, fmt.Errorf("validation: %w", err)
    }

    return data, nil
}

// Schema methods delegate to Registry
func (m *Marshaler) Schema(v any) json.RawMessage {
    if eventType, ok := m.reverse[elemType(v)]; ok {
        return m.registry.Schema(eventType)
    }
    return nil
}

func (m *Marshaler) Schemas() json.RawMessage {
    return m.registry.Schemas()
}

// NewInput: InputRegistry interface
func (m *Marshaler) NewInput(eventType string) any {
    if t, ok := m.types[eventType]; ok {
        return reflect.New(t).Interface()
    }
    return nil
}
```

**Usage Patterns:**

```go
// Pattern 1: Proxy (no types)
registry := jsonschema.NewRegistry()
registry.MustRegister("order.created", orderSchema)
registry.MustRegister("order.cancelled", cancelSchema)

validator := jsonschema.NewValidationMiddleware(registry)
pipe := pipe.NewPassthroughPipe(pipe.Config{})
pipe.Use(validator)
validated, _ := pipe.Pipe(ctx, rawInput)
publisher.Publish(ctx, validated)

// Pattern 2: Handlers (with types, shared registry)
marshaler := jsonschema.NewMarshaler(jsonschema.Config{
    Registry: registry,  // ✅ Share schemas!
    Naming:   message.DotNaming,
})
marshaler.Register(CreateOrder{}, orderSchema)  // Links type to "order.created"

engine := message.NewEngine(message.EngineConfig{
    Marshaler: marshaler,
})

// Pattern 3: Isolated (Marshaler creates its own Registry)
marshaler := jsonschema.NewMarshaler(jsonschema.Config{
    Naming: message.DotNaming,
})
marshaler.Register(CreateOrder{}, orderSchema)
// marshaler.registry is internal
```

**Pros:**
- ✅ Best of both worlds
- ✅ Registry handles core validation logic (type-agnostic)
- ✅ Marshaler adds type-safety layer
- ✅ Middleware supports proxy pattern
- ✅ Shared Registry between middleware and Marshaler
- ✅ Backward compatible API (Marshaler still works the same)
- ✅ Schema endpoints can use Registry directly

**Cons:**
- ⚠️ Two public types (Registry + Marshaler) - more API surface
- ⚠️ Need clear docs on when to use which

## Resolution

**Option A (Pure Separation)** was implemented, which turned out simpler than the originally recommended Option C:

- **No `jsonschema.Marshaler` wrapper** — marshaling uses standard `message.NewJSONMarshaler()`
- **Registry handles everything**: validation, Go type tracking, `InputRegistry`, schema serving
- **Three middleware types** for different pipe stages (input, output, proxy)
- **`RegisterType(v, schema)`** combines schema + Go type registration (Option A from Q2)

The Option C `Marshaler` wrapper was unnecessary because:
1. Registry already tracks Go types via `RegisterType` (no need for separate Marshaler layer)
2. Standard `message.NewJSONMarshaler()` handles serialization (no validation coupling needed)
3. Middleware handles validation at the pipe level (no need for marshal-time validation)

### Actual File Structure

```
message/jsonschema/
├── registry.go           # Core: eventType → schema validation + Go type tracking + InputRegistry
├── middleware.go          # Three middleware types for input/output/proxy validation
└── registry_test.go      # Comprehensive tests
```

## Resolved Questions

1. **Should Registry be in a separate package?**
   → No. Stayed in `message/jsonschema`. Single package is sufficient.

2. **Should Register accept schemaJSON, or just link types?**
   → Option A: `RegisterType(v, schema)` registers both together. Explicit `Register(eventType, schema)` without Go types deferred as future enhancement.

3. **Should we provide pre-composed middleware?**
   → Yes: three middleware constructors (`NewValidationMiddleware`, `NewInputValidationMiddleware`, `NewOutputValidationMiddleware`), each taking `*Registry`.

## Benefits for gopipe

1. **Separation of concerns**: Validation ≠ marshaling (fully decoupled)
2. **Proxy pattern**: Can validate without Go types (future: explicit `Register`)
3. **Middleware**: Idiomatic gopipe composition
4. **CloudEvents-native**: Registry uses eventType, not Go types
5. **Schema serving**: Individual + catalog endpoints via `Schema()` / `Schemas()`
6. **InputRegistry**: Registry implements it for automatic type creation in pipes

## Trade-offs

### Simplicity vs. Flexibility
- Only `RegisterType` (needs Go types), no `Register(eventType, schema)` yet
- Proxy pattern requires minimal Go types for now

**Mitigation**: `Register(eventType, schema)` can be added incrementally

### API Surface
- One public type (`Registry`) + three middleware constructors
- Simpler than originally planned Option C (which had Registry + Marshaler)
