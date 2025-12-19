# PRO-0020: Codec Serialization System

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0041

## Overview

Implement codec system for serialization at adapter boundaries.

## Goals

1. Define `Codec` interface
2. Implement `CodecRegistry`
3. Provide built-in codecs (JSON, Protobuf, Avro)

## Task

**Goal:** Create codec abstraction layer

**Files to Create:**
- `message/codec.go` - Codec interface
- `message/codec_registry.go` - CodecRegistry
- `message/codec_json.go` - JSON codec
- `message/codec_raw.go` - Raw passthrough codec

**Implementation:**
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

var JSONCodec Codec = &jsonCodec{}
var RawCodec Codec = &rawCodec{}
```

**Acceptance Criteria:**
- [ ] `Codec` interface defined
- [ ] `CodecRegistry` implemented
- [ ] JSON codec implemented
- [ ] Raw codec implemented
- [ ] Tests for encode/decode
- [ ] CHANGELOG updated

## Related

- [PRO-0041](../../adr/PRO-0041-codec-serialization.md) - ADR
