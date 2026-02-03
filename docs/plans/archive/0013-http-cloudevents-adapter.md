# Plan 0013: HTTP CloudEvents Adapter

**Status:** Complete
**Related ADRs:** [0024](../../adr/0024-http-cloudevents-adapter.md)

## Overview

HTTP pub/sub adapter for CloudEvents using standard library `net/http`. Provides HTTP-specific optimization complementing the generic `message/cloudevents` SDK wrapper.

## Goals

1. HTTP CloudEvents subscriber implementing `http.Handler`
2. HTTP CloudEvents publisher with batching support
3. Proper acking semantics (HTTP 2xx = Ack, errors = Nack)
4. Standard library integration (`http.ServeMux`, `http.Server`)
5. Support binary, structured, and batch CloudEvents modes

## Tasks

### Task 1: Subscriber Implementation

**Goal:** HTTP handler that converts CloudEvents requests to channel messages

**Implementation:**
- Implement `http.Handler` interface
- Parse CloudEvents using SDK (binary/structured/batch modes)
- Bridge acking to HTTP response codes
- Graceful shutdown with in-flight request tracking

**Files:**
- `message/http/subscriber.go`
- `message/http/subscriber_test.go`

### Task 2: Publisher Implementation

**Goal:** HTTP client that sends messages as CloudEvents with batching

**Implementation:**
- Send single messages synchronously (`Send`, `SendBatch`)
- Publish channel messages with automatic batching (`Publish`)
- Use CloudEvents SDK for serialization
- Batch mode using `pipe.BatchPipe`

**Files:**
- `message/http/publisher.go`
- `message/http/publisher_test.go`

### Task 3: Integration Tests

**Goal:** E2E tests for full HTTP pub/sub roundtrip

**Files:**
- `message/http/http_test.go`

### Task 4: Documentation and Example

**Goal:** Complete documentation and working example

**Files:**
- `message/http/doc.go`
- `examples/06-http-cloudevents/main.go`
- `CHANGELOG.md`

## Acceptance Criteria

- [x] Subscriber handles unlimited concurrent HTTP requests
- [x] Publisher supports single, stream, and batch modes
- [x] Acking correctly bridges to HTTP responses
- [x] Binary, structured, and batch modes supported
- [x] All unit tests pass
- [x] E2E test demonstrates full roundtrip
- [x] Example in `examples/` directory
- [x] Package documentation complete
- [x] CHANGELOG updated
