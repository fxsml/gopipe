# PRO-0024: Testing Infrastructure

**Status:** Proposed
**Priority:** Low
**Related ADRs:** PRO-0045

## Overview

Create testing infrastructure with testcontainers and mocks.

## Goals

1. Create broker container helpers
2. Create mock adapters
3. Integration test patterns

## Task 1: Test Containers

**Files to Create:**
- `internal/testing/containers.go`
- `internal/testing/nats.go`
- `internal/testing/kafka.go`
- `internal/testing/rabbitmq.go`

```go
type BrokerContainer interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ConnectionURL() string
}

func WithBroker(t *testing.T, broker BrokerContainer, f func(url string))
```

## Task 2: Mock Adapters

**Files to Create:**
- `internal/testing/mock_subscriber.go`
- `internal/testing/mock_publisher.go`

```go
type MockSubscriber struct {
    messages chan *message.Message
}

func NewMockSubscriber() *MockSubscriber
func (s *MockSubscriber) Subscribe(ctx context.Context) <-chan *message.Message
func (s *MockSubscriber) Send(msg *message.Message)

type MockPublisher struct {
    Published []PublishedMessage
}

func NewMockPublisher() *MockPublisher
func (p *MockPublisher) Publish(ctx context.Context, topic string, msg *message.Message) error
```

**Acceptance Criteria:**
- [ ] BrokerContainer interface
- [ ] NATS container helper
- [ ] Kafka container helper
- [ ] MockSubscriber implemented
- [ ] MockPublisher implemented
- [ ] Example integration tests
- [ ] CHANGELOG updated

## Related

- [PRO-0045](../../adr/PRO-0045-testing-infrastructure.md) - ADR
