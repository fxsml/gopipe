# ADR 0045: Testing Infrastructure

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #9)

## Context

Integration tests require running brokers. Need testing infrastructure with testcontainers and mock adapters.

## Decision

### Test Containers

```go
// internal/testing/brokers.go

type BrokerContainer interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ConnectionURL() string
}

func NewNATSContainer() BrokerContainer
func NewKafkaContainer() BrokerContainer
func NewRabbitMQContainer() BrokerContainer

// Integration test helper
func WithBroker(t *testing.T, broker BrokerContainer, f func(url string)) {
    ctx := context.Background()
    if err := broker.Start(ctx); err != nil {
        t.Skipf("skipping: %v", err)
    }
    defer broker.Stop(ctx)
    f(broker.ConnectionURL())
}
```

**Usage:**

```go
func TestKafkaAdapter_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    WithBroker(t, NewKafkaContainer(), func(url string) {
        sub := kafka.NewSubscriber(kafka.SubscriberConfig{
            Brokers: []string{url},
            Topics:  []string{"test"},
        })
        // ... test logic
    })
}
```

### Mock Adapters

```go
// internal/testing/mock.go

type MockSubscriber struct {
    messages chan *message.Message
    closed   bool
}

func NewMockSubscriber() *MockSubscriber
func (s *MockSubscriber) Subscribe(ctx context.Context) <-chan *message.Message
func (s *MockSubscriber) Send(msg *message.Message)
func (s *MockSubscriber) Close()

type MockPublisher struct {
    Published []PublishedMessage
    mu        sync.Mutex
}

func NewMockPublisher() *MockPublisher
func (p *MockPublisher) Publish(ctx context.Context, topic string, msg *message.Message) error
func (p *MockPublisher) Messages() []PublishedMessage
func (p *MockPublisher) MessagesForTopic(topic string) []*message.Message
```

## Consequences

**Positive:**
- Real broker testing
- Controllable mocks
- Skip integration in short mode

**Negative:**
- Docker dependency
- Test setup complexity

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)
