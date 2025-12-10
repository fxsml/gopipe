# Issues Found

All issues have been resolved in commit `a695139`.

## Status Summary

| Issue | Status | Resolution |
|-------|--------|------------|
| #0 Duplicate brokers | ✅ Resolved | Removed InMemoryBroker, kept unified Broker |
| #1 Concrete types | ✅ Resolved | All constructors return concrete types |
| #2 Topic separator | ✅ Resolved | Standardized on `/`, exact match only |
| #3 HTTP message clearing | ✅ Resolved | Messages cleared after Receive() |
| #4 HTTP ack handling | ✅ Resolved | Added `WaitForAck` config |
| #5 Subscription IDs | ✅ Resolved | Uses map with atomic IDs |
| #6 Feedback loop | ✅ Non-issue | Already works correctly |
| #7 Pattern matching | ✅ Resolved | Removed, exact match only |

---

## 0. InMemoryBroker vs ChannelBroker ✅ RESOLVED

**Resolution**: Removed both, created unified `Broker` type in `pubsub/broker.go`.

```go
// New unified Broker
type Broker struct {
    config BrokerConfig
    mu     sync.RWMutex
    subs   map[string]*subscription  // keyed by unique ID
    nextID uint64
    closed bool
}

// Returns concrete type
func NewBroker(config BrokerConfig) *Broker

// Subscribe returns channel (primary API)
func (b *Broker) Subscribe(ctx context.Context, topic string) <-chan *message.Message

// Send fans out to exact topic matches
func (b *Broker) Send(ctx context.Context, topic string, msgs []*message.Message) error

// Receive creates temp subscription for polling (compatibility API)
func (b *Broker) Receive(ctx context.Context, topic string) ([]*message.Message, error)
```

Files deleted:
- `pubsub/memory.go` (InMemoryBroker)
- `pubsub/channel.go` (ChannelBroker)

---

## 1. Broker Constructors Return Interfaces ✅ RESOLVED

**Resolution**: All constructors now return concrete types with compile-time interface assertions.

```go
// pubsub/broker.go
var (
    _ Sender   = (*Broker)(nil)
    _ Receiver = (*Broker)(nil)
)
func NewBroker(config BrokerConfig) *Broker

// pubsub/http.go
var _ Sender = (*HTTPSender)(nil)
var _ Receiver = (*HTTPReceiver)(nil)
func NewHTTPSender(url string, config HTTPConfig) *HTTPSender
func NewHTTPReceiver(config HTTPConfig, bufferSize int) *HTTPReceiver

// pubsub/io.go
var (
    _ Sender   = (*IOBroker)(nil)
    _ Receiver = (*IOBroker)(nil)
)
func NewIOBroker(r io.Reader, w io.Writer, config IOConfig) *IOBroker
```

---

## 2. Topic Separator Mismatch ✅ RESOLVED

**Resolution**: Standardized on `/` separator throughout. Removed wildcard patterns.

| Component | Separator | Example |
|-----------|-----------|---------|
| Broker | `/` | `orders/created` |
| HTTPReceiver | `/` | `commands/create-order` |
| Multiplex | `/` | `internal/cache` |
| Topics utility | `/` | `SplitTopic()`, `JoinTopic()` |

---

## 3. HTTPReceiver.Receive() Never Clears Messages ✅ RESOLVED

**Resolution**: Added `readIndex` tracking per topic. Messages cleared after Receive().

```go
type HTTPReceiver struct {
    messages   map[string][]topicMessage
    readIndex  map[string]int  // track read position per topic
}

func (r *HTTPReceiver) Receive(ctx, topic string) ([]*message.Message, error) {
    msgs := r.messages[topic]
    start := r.readIndex[topic]
    // Return unread messages
    result := msgs[start:]
    r.readIndex[topic] = len(msgs)
    // Clean up if all read
    if r.readIndex[topic] >= len(msgs) {
        delete(r.messages, topic)
        delete(r.readIndex, topic)
    }
    return result, nil
}
```

---

## 4. HTTP Response Code Handling ✅ RESOLVED

**Resolution**: Added `WaitForAck` and `AckTimeout` to HTTPConfig.

```go
type HTTPConfig struct {
    // WaitForAck makes the HTTP receiver wait for message acknowledgment.
    // When true: returns 200 OK after ack, 500 on nack/timeout.
    // When false: returns 201 Created immediately.
    WaitForAck bool

    // AckTimeout is the maximum duration to wait for acknowledgment.
    // Default: 30 seconds.
    AckTimeout time.Duration
}
```

Note: Full Ack/Nack channel support deferred to future iteration.

---

## 5. ChannelBroker Subscription Index Corruption ✅ RESOLVED

**Resolution**: Uses `map[string]*subscription` with atomic ID generation.

```go
type Broker struct {
    subs   map[string]*subscription  // keyed by subscription ID
    nextID uint64
}

func (b *Broker) nextSubID() string {
    id := atomic.AddUint64(&b.nextID, 1)
    return string(rune(id)) + "-sub"
}

// In Subscribe:
id := b.nextSubID()
b.subs[id] = sub

// In cleanup:
delete(b.subs, id)  // Safe - doesn't affect other subscriptions
```

---

## 6. Router Output Feedback Loop ✅ NON-ISSUE

Already works correctly:

```go
// Subscribe to broker
msgs := broker.Subscribe(ctx, "commands")

// Router processes messages
output := router.Start(ctx, msgs)

// Publisher sends output back to broker
done := publisher.Publish(ctx, output)

<-done
```

---

## 7. Multiplex Pattern Matching Complexity ✅ RESOLVED

**Resolution**: Removed all pattern matching. Exact topic match only.

```go
// Before (removed)
{Pattern: "internal.*", Sender: memoryBroker}

// After (exact match)
{Topic: "internal/cache", Sender: memoryBroker}

// Prefix matching still available via helper
selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
```

Files changed:
- `pubsub/topics.go`: Removed `TopicMatcher`, kept only utility functions
- `pubsub/multiplex.go`: `Pattern` field renamed to `Topic`, removed `matchTopicPattern()`
