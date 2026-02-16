# Validation vs. Marshaling Separation - Design Evolution

**Status:** Open
**Related Plan:** validating-marshaler-example-enhancement.md

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
    Naming:   message.KebabNaming,
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
    Naming:   message.KebabNaming,
})
marshaler.Register(CreateOrder{}, orderSchema)  // Links type to "order.created"

engine := message.NewEngine(message.EngineConfig{
    Marshaler: marshaler,
})

// Pattern 3: Isolated (Marshaler creates its own Registry)
marshaler := jsonschema.NewMarshaler(jsonschema.Config{
    Naming: message.KebabNaming,
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

## Recommendation

**Option C (Hybrid)** is the best solution:

1. **Registry** is the core - validation by CloudEvents type (no Go types)
2. **Marshaler** is a typed wrapper around Registry
3. **Middleware** uses Registry directly for proxy scenarios
4. **Backward compatible** - existing Marshaler API unchanged

### Migration Path

1. Extract Registry from current Marshaler implementation
2. Refactor Marshaler to use Registry internally
3. Add NewValidationMiddleware(registry)
4. Update example to show both patterns
5. Fix schema endpoint to use Registry (supports both Go type name and eventType)

### File Structure

```
message/jsonschema/
├── registry.go           # Core: eventType → schema validation
├── marshaler.go          # Wrapper: Go type → eventType + Registry
├── middleware.go         # Validation middleware for RawMessage
├── registry_test.go
├── marshaler_test.go
├── middleware_test.go
└── pipe.go              # UnmarshalPipe, MarshalPipe (use Marshaler)
```

## Benefits for gopipe

1. **Separation of concerns**: Validation ≠ marshaling
2. **Proxy pattern**: Validate without types (HTTP → AMQP, etc.)
3. **Middleware**: Idiomatic gopipe composition
4. **Type-safety**: Marshaler preserves current type-safe API
5. **Reusability**: Registry shared across components
6. **CloudEvents-native**: Registry uses eventType, not Go types

## Trade-offs

### Complexity
- **Before**: 1 component (Marshaler)
- **After**: 3 components (Registry, Marshaler, Middleware)

**Mitigation**: Clear docs, examples for each use case

### API Surface
- More types means more to document/maintain

**Mitigation**: Registry is the primitive, others are convenience wrappers

### Breaking Changes
- If we refactor current Marshaler's internal storage from `map[reflect.Type]` to `map[string]`

**Mitigation**: Keep Marshaler API unchanged, internals can change

## Open Questions

1. **Should Registry be in a separate package?** (e.g., `message/jsonschema/registry`)
   - Pro: Clearer separation
   - Con: More imports

2. **Should Marshaler.Register still accept schemaJSON, or just link types?**
   ```go
   // Option A: Current (register schema + type together)
   marshaler.Register(CreateOrder{}, schema)

   // Option B: Separate registration
   registry.Register("order.created", schema)
   marshaler.RegisterType(CreateOrder{})  // Links to "order.created"
   ```

3. **Should we provide pre-composed middleware?**
   ```go
   // Instead of requiring users to create Registry + Middleware
   validator := jsonschema.NewValidator(eventType, schema, ...)
   pipe.Use(validator)
   ```

## Next Steps

1. Get user feedback on Option C
2. Create ADR documenting the decision
3. Implement Registry extraction
4. Add validation middleware
5. Update examples to show both patterns
6. Update schema endpoints to use Registry
