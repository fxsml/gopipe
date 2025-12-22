# Plan 0021: Marshaler Implementation

**ADR:** [0021-codec-marshaling-pattern.md](../adr/0021-codec-marshaling-pattern.md)
**Status:** Draft

## Overview

Implement a Marshaler with bidirectional type registry for CloudEvents type ↔ Go type mapping.

---

## Core Interface

```go
// Marshaler handles serialization and type mapping
type Marshaler interface {
    // Register maps a CloudEvents type to a Go type prototype
    Register(ceType string, prototype any)

    // Name returns the CloudEvents type for a Go value
    Name(v any) string

    // TypeFor returns the Go type for a CloudEvents type
    TypeFor(ceType string) (reflect.Type, bool)

    // Marshal converts a Go value to []byte
    Marshal(v any) ([]byte, error)

    // Unmarshal converts []byte to a Go value based on CloudEvents type
    Unmarshal(data []byte, ceType string) (any, error)

    // ContentType returns the MIME type
    ContentType() string
}
```

---

## JSON Marshaler Implementation

```go
type JSONMarshaler struct {
    mu       sync.RWMutex
    ceToGo   map[string]reflect.Type  // "order.created" → Order
    goToCE   map[reflect.Type]string  // Order → "order.created"
    naming   NamingStrategy
}

func NewJSONMarshaler() *JSONMarshaler {
    return &JSONMarshaler{
        ceToGo: make(map[string]reflect.Type),
        goToCE: make(map[reflect.Type]string),
        naming: NamingKebab,
    }
}

func (m *JSONMarshaler) Register(ceType string, prototype any) {
    t := reflect.TypeOf(prototype)
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }

    m.mu.Lock()
    defer m.mu.Unlock()
    m.ceToGo[ceType] = t
    m.goToCE[t] = ceType
}

func (m *JSONMarshaler) RegisterAll(types map[string]any) {
    for ceType, proto := range types {
        m.Register(ceType, proto)
    }
}

func (m *JSONMarshaler) Name(v any) string {
    t := reflect.TypeOf(v)
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }

    m.mu.RLock()
    name, ok := m.goToCE[t]
    m.mu.RUnlock()

    if ok {
        return name
    }
    return m.naming(t)
}

func (m *JSONMarshaler) TypeFor(ceType string) (reflect.Type, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    t, ok := m.ceToGo[ceType]
    return t, ok
}

func (m *JSONMarshaler) Unmarshal(data []byte, ceType string) (any, error) {
    t, ok := m.TypeFor(ceType)
    if !ok {
        // Unknown type - return raw bytes
        return data, nil
    }

    // Create new instance
    v := reflect.New(t).Interface()
    if err := json.Unmarshal(data, v); err != nil {
        return nil, fmt.Errorf("unmarshal %s: %w", ceType, err)
    }

    // Return value, not pointer
    return reflect.ValueOf(v).Elem().Interface(), nil
}

func (m *JSONMarshaler) Marshal(v any) ([]byte, error) {
    return json.Marshal(v)
}

func (m *JSONMarshaler) ContentType() string {
    return "application/json"
}

func (m *JSONMarshaler) SetNaming(strategy NamingStrategy) {
    m.naming = strategy
}
```

---

## Naming Strategies

```go
type NamingStrategy func(t reflect.Type) string

// NamingSimple uses just the type name
// OrderCreated → "OrderCreated"
var NamingSimple NamingStrategy = func(t reflect.Type) string {
    return t.Name()
}

// NamingKebab converts to kebab-case with dots
// OrderCreated → "order.created"
var NamingKebab NamingStrategy = func(t reflect.Type) string {
    name := t.Name()
    var result strings.Builder
    for i, r := range name {
        if i > 0 && unicode.IsUpper(r) {
            result.WriteRune('.')
        }
        result.WriteRune(unicode.ToLower(r))
    }
    return result.String()
}

// NamingPackageQualified includes package name
// github.com/org/events.OrderCreated → "events.OrderCreated"
var NamingPackageQualified NamingStrategy = func(t reflect.Type) string {
    pkg := t.PkgPath()
    if idx := strings.LastIndex(pkg, "/"); idx >= 0 {
        pkg = pkg[idx+1:]
    }
    if pkg == "" {
        return t.Name()
    }
    return pkg + "." + t.Name()
}
```

---

## Usage Examples

### Explicit Registration

```go
marshaler := message.NewJSONMarshaler()
marshaler.Register("order.created", OrderCreated{})
marshaler.Register("order.updated", OrderUpdated{})
marshaler.Register("order.cancelled", OrderCancelled{})
```

### Bulk Registration

```go
marshaler.RegisterAll(map[string]any{
    "order.created":    OrderCreated{},
    "order.updated":    OrderUpdated{},
    "payment.received": PaymentReceived{},
})
```

### Auto-Naming (No Registration)

```go
marshaler := message.NewJSONMarshaler()
marshaler.SetNaming(NamingKebab)

// OrderCreated{} automatically maps to "order.created"
name := marshaler.Name(OrderCreated{})  // "order.created"
```

---

## Future: Composite Marshaler

```go
type CompositeMarshaler struct {
    marshalers map[string]Marshaler  // content-type → marshaler
    fallback   Marshaler
}

func NewCompositeMarshaler(fallback Marshaler) *CompositeMarshaler {
    return &CompositeMarshaler{
        marshalers: make(map[string]Marshaler),
        fallback:   fallback,
    }
}

func (m *CompositeMarshaler) RegisterCodec(contentType string, marshaler Marshaler) {
    m.marshalers[contentType] = marshaler
}

func (m *CompositeMarshaler) ForContentType(ct string) Marshaler {
    if marshaler, ok := m.marshalers[ct]; ok {
        return marshaler
    }
    return m.fallback
}
```

---

## Test Plan

1. Register type, unmarshal returns typed value
2. Unregistered type returns raw `[]byte`
3. Name() returns registered CE type
4. Name() falls back to naming strategy for unregistered types
5. Round-trip: Marshal then Unmarshal preserves data
6. NamingKebab converts correctly (OrderCreated → order.created)
7. Thread-safe concurrent registration
