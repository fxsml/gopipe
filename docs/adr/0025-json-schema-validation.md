# ADR 0025: JSON Schema Validation

**Date:** 2026-02-16
**Status:** Implemented

## Context

Message validation is essential for event-driven architectures. Go's zero-value semantics create a validation gap: `encoding/json` fills missing required fields with zero values (`""`, `0`, `false`), making it impossible to distinguish between explicitly set zeros and missing fields.

JSON Schema solves this by validating the raw JSON **before** unmarshaling. This catches missing required fields, type mismatches, and constraint violations at the system boundary.

**Requirements:**
1. Validate CloudEvents message payloads against JSON Schema definitions
2. Support both typed scenarios (handlers with Go types) and proxy scenarios (no types)
3. Separate validation concerns from serialization concerns
4. Integrate with gopipe's middleware pattern
5. Serve schemas via HTTP for API contract discovery

## Decision

Implement JSON Schema validation through a `Registry` + middleware pattern, separating validation from marshaling.

### Core: Registry

`Registry` provides schema validation by CloudEvents type, with optional Go type tracking:

```go
type Registry struct {
    schemas  map[string]*entry          // eventType → schema
    types    map[string]reflect.Type    // eventType → Go type (optional)
    reverse  map[reflect.Type]string    // Go type → eventType
    naming   message.EventTypeNaming
}

type Config struct {
    Naming    message.EventTypeNaming              // Default: KebabNaming
    SchemaURI func(eventType string) string        // Default: urn:gopipe:schema:cloudevents:{eventType}
}

func NewRegistry(cfg Config) *Registry
func (r *Registry) RegisterType(v any, schemaJSON string) error
func (r *Registry) MustRegisterType(v any, schemaJSON string)
func (r *Registry) Validate(eventType string, data []byte) error
func (r *Registry) NewInput(eventType string) any  // InputRegistry
func (r *Registry) Schema(eventType string) json.RawMessage
func (r *Registry) Schemas() json.RawMessage
```

**Key properties:**
- Validates by **CloudEvents type**, not Go type
- RegisterType uses naming strategy to derive eventType automatically
- Implements InputRegistry for UnmarshalPipe support
- Thread-safe for shared use across middleware/pipes
- **No marshaling** - pure validation only

### Validation: Middleware

Three middleware types for different pipeline stages:

```go
// Proxy: RawMessage → RawMessage (no unmarshaling)
func NewValidationMiddleware(registry *Registry)
    middleware.Middleware[*RawMessage, *RawMessage]

// Pre-validation: Before unmarshaling (fail fast)
func NewInputValidationMiddleware(registry *Registry)
    middleware.Middleware[*RawMessage, *Message]

// Post-validation: After marshaling (validate output)
func NewOutputValidationMiddleware(registry *Registry)
    middleware.Middleware[*Message, *RawMessage]
```

### Marshaling: Standard JSON

Use standard `message.NewJSONMarshaler()` for serialization. Validation is separate:

```go
marshaler := message.NewJSONMarshaler()
unmarshalPipe := message.NewUnmarshalPipe(registry, marshaler, cfg)
unmarshalPipe.Use(jsonschema.NewInputValidationMiddleware(registry))
```

## Usage Patterns

### Pattern 1: Typed Handlers

```go
registry := jsonschema.NewRegistry(jsonschema.Config{
    Naming: message.KebabNaming,
})
registry.MustRegisterType(CreateOrder{}, orderSchema)
// Derives "create.order" automatically

marshaler := message.NewJSONMarshaler()
unmarshalPipe := message.NewUnmarshalPipe(registry, marshaler, cfg)
unmarshalPipe.Use(jsonschema.NewInputValidationMiddleware(registry))
```

### Pattern 2: Proxy (No Types)

For proxy scenarios, explicit `Register(eventType, schema)` can be added later. For now, proxy scenarios can define minimal Go types purely for validation without actual unmarshaling.

### Pattern 3: Schema Serving

```go
// Individual schema by CloudEvents type
mux.HandleFunc("GET /schema/{type}", func(w http.ResponseWriter, r *http.Request) {
    eventType := r.PathValue("type")
    schema := registry.Schema(eventType)  // e.g., "create.order"
    w.Header().Set("Content-Type", "application/schema+json")
    w.Write(schema)
})

// Catalog of all schemas
mux.HandleFunc("GET /schemas", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/schema+json")
    w.Write(registry.Schemas())  // Composed $defs document
})
```

## Consequences

### Benefits

1. **Pure separation** - Validation and marshaling are independent concerns
2. **Middleware pattern** - Idiomatic gopipe composition
3. **Proxy support** - Can validate without Go types (future: explicit Register)
4. **CloudEvents-native** - Validation by eventType, not Go types
5. **Schema serving** - Registry provides both individual and catalog endpoints
6. **Fail fast** - Pre-validation catches errors before unmarshaling
7. **Type-safe** - Go type tracking via RegisterType for handlers
8. **Reusable** - Single Registry shared across middleware and pipes

### Breaking Changes

- New package: `message/jsonschema`
- Requires JSON Schema Draft 2020-12 format
- Depends on `github.com/santhosh-tekuri/jsonschema/v6`

### Limitations

1. **RegisterType only** - Explicit `Register(eventType, schema)` deferred to keep initial API focused
2. **Single schema per type** - One eventType maps to one schema (versioning handled via new types)
3. **Validation overhead** - Additional parsing/validation on hot path (mitigated by pre-compilation)

### Design Decisions

**Why Registry, not Marshaler?**
- "Marshaler" implies serialization responsibility (Marshal/Unmarshal methods)
- Registry focuses on validation only, avoiding conceptual coupling
- Standard JSON marshaling is separate (message.NewJSONMarshaler)

**Why RegisterType only initially?**
- Simpler API surface during initial release
- Justifies InputRegistry implementation (needs Go types)
- Explicit Register(eventType, schema) can be added incrementally
- Covers 90% of use cases (typed handlers)

**Why middleware instead of Marshaler.Unmarshal validation?**
- Middleware is composable across different pipe stages
- Allows pre-validation before unmarshaling (fail fast)
- Separates concerns: validation vs serialization
- Enables proxy scenarios without unmarshaling

**Why three middleware types?**
- Different pipe stages have different signatures
- InputValidationMiddleware validates before unmarshaling
- OutputValidationMiddleware validates after marshaling
- ValidationMiddleware for proxy scenarios (RawMessage → RawMessage)
- Each serves a distinct use case

**Why CloudEvents type, not Go type?**
- CloudEvents type is the runtime identity
- Same Go type might represent different event versions
- Proxy scenarios have no Go types
- Aligns with message routing and InputRegistry

## Implementation Notes

### Schema URI Convention

Schemas are compiled with URIs derived from CloudEvents types. By default:

```go
func defaultSchemaURI(eventType string) string {
    return fmt.Sprintf("urn:gopipe:schema:cloudevents:%s", eventType)
}
```

Custom URI schemes can be configured for HTTP-based schemas or organization-specific naming:

```go
registry := jsonschema.NewRegistry(jsonschema.Config{
    SchemaURI: func(eventType string) string {
        return fmt.Sprintf("https://api.example.com/schemas/%s", eventType)
    },
})
```

The URI is used internally by the JSON Schema compiler and doesn't need to be resolvable. However, using HTTP URLs allows schemas to be published at those locations for external tooling or CloudEvents `dataschema` attribute references.

### Thread Safety

Registry is thread-safe via RWMutex, allowing:
- Concurrent validation across goroutines
- Shared Registry between middleware and pipes
- One-time registration at startup, many readers during runtime

### Validation Pass-Through

If no schema is registered for an eventType, `Validate()` returns `nil` (pass-through). This allows gradual schema adoption without breaking unvalidated messages.

## Alternatives Considered

### Alternative 1: Validation in Marshaler

Keep validation coupled to Marshal/Unmarshal methods:

```go
type Marshaler struct { ... }
func (m *Marshaler) Unmarshal(data []byte, v any) error {
    // Validate, then unmarshal
}
```

**Rejected because:**
- Couples validation with serialization (violates single responsibility)
- Doesn't support proxy scenarios (need types to marshal)
- Less flexible than middleware composition
- Harder to validate at different pipeline stages

### Alternative 2: Single Middleware Type

Use `Message` everywhere instead of RawMessage/Message distinction:

```go
type ValidationMiddleware = Middleware[*Message, *Message]
```

**Deferred because:**
- Requires broader architectural change (Message unification)
- Current RawMessage vs Message distinction is intentional
- Can be revisited later if Message types are unified
- Three middleware types work well with current architecture

### Alternative 3: Engine Integration

Build validation directly into Engine:

```go
engine := message.NewEngine(message.EngineConfig{
    Validator: jsonschema.NewValidator(...),
})
```

**Rejected because:**
- Less flexible than middleware approach
- Couples validation to Engine (not all pipes use Engine)
- Middleware is more composable and testable
- Users can add validation exactly where needed

## Links

- Related: ADR 0021 (Marshaler and NamingStrategy)
- Related: ADR 0022 (Message Package Redesign)
- Related: ADR 0024 (HTTP CloudEvents Adapter)
- Plan: [validating-marshaler-example-enhancement.md](../plans/validating-marshaler-example-enhancement.md)
- Implementation: `message/jsonschema/registry.go`
- Example: `examples/07-validating-marshaler/`

## Future Enhancements

1. **Explicit Register** - Add `Register(eventType, schema)` for pure proxy scenarios
2. **Schema versioning** - Convention for versioned schemas (e.g., `order.created.v2`)
3. **Custom validators** - Plugin system for domain-specific validation
4. **Validation caching** - Cache validation results for identical payloads
5. **Schema references** - Support `$ref` across registered schemas
6. **Metrics** - Validation success/failure counters for observability
