# ADR 0041: Codec Serialization System

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #5)

## Context

Where should serialization happen - application or adapter? Need clear boundary with automatic codec selection based on `datacontenttype`.

## Decision

Introduce `Codec` interface at adapter boundaries:

```go
type Codec interface {
    ContentType() string
    Encode(v any) ([]byte, error)
    Decode(data []byte, v any) error
}

type CodecRegistry struct {
    codecs  map[string]Codec
    default Codec
}

func NewCodecRegistry() *CodecRegistry
func (r *CodecRegistry) Register(codec Codec)
func (r *CodecRegistry) Get(contentType string) Codec
func (r *CodecRegistry) SetDefault(codec Codec)

// Built-in codecs
var (
    JSONCodec     Codec = &jsonCodec{}
    ProtobufCodec Codec = &protobufCodec{}
    AvroCodec     Codec = &avroCodec{}
    RawCodec      Codec = &rawCodec{}
)

type AdapterCodecConfig struct {
    Registry           *CodecRegistry
    DefaultContentType string
    ValidateOnEncode   bool
    SchemaRegistry     string
}
```

**TypedSubscriber for automatic deserialization:**

```go
type TypedSubscriber[T any] struct {
    subscriber Subscriber
    codec      Codec
}

func NewTypedSubscriber[T any](sub Subscriber, codec Codec) *TypedSubscriber[T]
func (s *TypedSubscriber[T]) Subscribe(ctx context.Context) <-chan *TypedMessage[T]
```

## Consequences

**Positive:**
- Clear serialization boundary
- Automatic codec selection
- Schema registry support

**Negative:**
- Additional abstraction
- Runtime type assertions

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)
