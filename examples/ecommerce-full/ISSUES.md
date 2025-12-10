# Issues Found

## 0. InMemoryBroker vs ChannelBroker

We have two in-process brokers with overlapping purposes. Only one should be kept.

### Comparison

| Feature | InMemoryBroker | ChannelBroker | Watermill GoChannel |
|---------|---------------|---------------|---------------------|
| Storage model | Slice (persists forever) | Channels (transient) | Both (configurable) |
| Delivery | Pull only | Push + Pull | Push (Subscribe) |
| `Subscribe()` | ❌ No | ✅ Yes | ✅ Yes |
| Pattern matching | ❌ No | ✅ Yes (buggy) | ❌ No |
| Message fate if no subscriber | Persists in memory | Lost | Configurable |
| Ack/Nack | ❌ No | ❌ No | ✅ Yes |
| Message clearing | Never (bug) | On delivery | On Ack |

### InMemoryBroker Problems
```go
// Send: appends to slice
func (b *inMemoryBroker) Send(...) { t.addMessage(msg) }

// Receive: returns ALL messages, never clears!
func (b *inMemoryBroker) Receive(...) { return t.getMessages() }  // copy, no clear
```
- Messages accumulate forever until `Close()`
- No way to acknowledge/remove processed messages
- Pull-only, no `Subscribe()` for real-time use

### ChannelBroker Problems
- Index-based subscription removal is buggy (see issue #5)
- Pattern matching adds complexity
- Messages lost if no active subscriber

### Recommendation: **Keep ChannelBroker, Remove InMemoryBroker**

Reasons:
1. Push-based delivery aligns with event-driven patterns
2. Has `Subscribe()` method for real-time messaging
3. Closer to Watermill's GoChannel design
4. InMemoryBroker's "never clear" behavior is a footgun

### Proposed Changes to ChannelBroker

```go
// Rename to just "Broker" since it's the only in-process broker
type Broker struct {
    config BrokerConfig
    mu     sync.RWMutex
    subs   map[string]*subscription  // keyed by unique ID, not slice index
    nextID uint64
    closed bool
}

type BrokerConfig struct {
    BufferSize   int
    SendTimeout  time.Duration
    CloseTimeout time.Duration
    // Optional: Persistent bool  // if true, store messages for late subscribers
}

// Return concrete type
func NewBroker(config BrokerConfig) *Broker { ... }

// Subscribe returns channel (main API)
func (b *Broker) Subscribe(ctx context.Context, topic string) <-chan *message.Message

// Send fans out to matching subscribers
func (b *Broker) Send(ctx context.Context, topic string, msgs []*message.Message) error

// Receive removed or deprecated - use Subscribe instead
```

**NOTE**: its fine to implement Subscribe and then consequently Publish. but send and receive should also be implemented. however, publish/subscribe may be the main functions.

### Alternative: Add Persistence Option (like Watermill)

If we need InMemoryBroker's persistence behavior:

```go
type BrokerConfig struct {
    // Persistent stores messages and delivers to late subscribers
    Persistent bool
}
```

When `Persistent: true`:
- Store messages in slice per topic
- New subscribers receive all past messages
- Messages cleared only on explicit `Clear(topic)` or `Close()`

---

## 1. Broker Constructors Return Interfaces

**Problem**: `NewHTTPReceiver()` returns `Receiver` interface, hiding the concrete type.

```go
httpReceiver := pubsub.NewHTTPReceiver(config, 100)
// Can't use httpReceiver as http.Handler!
// The concrete type has ServeHTTP but it's hidden behind Receiver interface
```

**Solution**: Return concrete types with compile-time interface assertion.

```go
type HTTPReceiver struct { ... }

var _ Receiver = (*HTTPReceiver)(nil) // compile-time check

func NewHTTPReceiver(config HTTPConfig, bufferSize int) *HTTPReceiver {
    return &HTTPReceiver{...}
}
```

---

## 2. Topic Separator Mismatch

**Problem**: Different components use different separators.

| Component | Separator | Example |
|-----------|-----------|---------|
| HTTPReceiver | `/` | `commands/create-order` |
| Multiplex selectors | `.` | `commands.create-order` |
| TopicMatcher | `/` | `commands/+/created` |

**Solution**: Standardize on `/` separator. For v1, support exact match only (remove wildcard patterns).

---

## 3. HTTPReceiver.Receive() Never Clears Messages

**Problem**: Messages accumulate forever. Each `Receive()` call returns all messages ever received.

```go
// First call
msgs, _ := receiver.Receive(ctx, "topic") // returns [msg1, msg2]

// Second call - same messages returned!
msgs, _ := receiver.Receive(ctx, "topic") // returns [msg1, msg2] again
```

**Solution**: Track read position per topic, return only unread messages.

```go
type HTTPReceiver struct {
    messages  []topicMessage
    readIndex map[string]int  // track position per topic
}

func (r *HTTPReceiver) Receive(ctx, topic string) ([]*message.Message, error) {
    start := r.readIndex[topic]
    // ... filter from start
    r.readIndex[topic] = len(r.messages)
    return result, nil
}
```

**NOTE**: we need to ensure that for the http receier (and also the channel receiver), we release resources after usage. no need to keep history.

---

## 4. HTTP Response Code Handling

**Problem**: HTTPReceiver always returns 201 immediately. No way to wait for message processing before responding.

**Desired behavior** (inspired by Watermill):
- `201 Created`: Fire-and-forget, respond immediately
- `200 OK`: Wait until message is acknowledged before responding

**Solution**: Add config option and acknowledgment channel.

```go
type HTTPConfig struct {
    // WaitForAck blocks HTTP response until message is acknowledged.
    // When true, returns 200 on ack, 500 on nack/timeout.
    // When false, returns 201 immediately.
    WaitForAck bool
    AckTimeout time.Duration
}
```

The message would include an ack channel:
```go
msg := message.New(body, props)
msg.SetAckChannel(ackCh) // internal method

// In handler, after sending to output:
if config.WaitForAck {
    select {
    case <-msg.Acked():
        w.WriteHeader(http.StatusOK)
    case <-msg.Nacked():
        w.WriteHeader(http.StatusInternalServerError)
    case <-time.After(config.AckTimeout):
        w.WriteHeader(http.StatusGatewayTimeout)
    }
} else {
    w.WriteHeader(http.StatusCreated)
}
```

---

## 5. ChannelBroker Subscription Index Corruption

**Problem**: Subscription removal uses slice index which becomes invalid when other subscriptions are removed.

```go
// In Receive()
subIndex := len(b.subscriptions) - 1

// Later in defer - but index may be wrong!
b.subscriptions = append(b.subscriptions[:subIndex], b.subscriptions[subIndex+1:]...)
```

**Solution**: Use map with unique IDs instead of slice indices.

```go
type channelBroker struct {
    subscriptions map[string]*channelSubscription  // key = unique ID
    nextID        uint64
}

func (b *channelBroker) addSubscription(sub *channelSubscription) string {
    id := fmt.Sprintf("sub-%d", atomic.AddUint64(&b.nextID, 1))
    b.subscriptions[id] = sub
    return id
}

func (b *channelBroker) removeSubscription(id string) {
    delete(b.subscriptions, id)
}
```

---

## 6. Router Output Feedback Loop

**Non-issue**: Router and Publisher already have compatible signatures.

```go
// Router.Start returns <-chan *message.Message
// Publisher.Publish accepts <-chan *message.Message
// They connect directly:

routerOutput := router.Start(ctx, inputCh)
done := publisher.Publish(ctx, routerOutput)  // Direct connection!
```

The cascading flow:
```
Broker.Subscribe() → Router.Start() → Publisher.Publish() → Broker.Send()
       ↑                                                          │
       └──────────────────────────────────────────────────────────┘
```

**Example** (correct wiring):
```go
// Subscribe to broker
msgs := broker.Subscribe(ctx, "commands/#")

// Router processes messages, handlers return new messages
output := router.Start(ctx, msgs)

// Publisher sends handler output back to broker
done := publisher.Publish(ctx, output)

<-done // wait for completion
```

No helper function needed.

---

## 7. Multiplex Pattern Matching Complexity

**Problem**: Wildcard patterns (`*`, `**`, `+`, `#`) add complexity without clear use cases for v1.

**Solution**: Remove pattern matching for v1. Support exact topic match only.

```go
// Before: complex pattern matching
selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
    {Pattern: "internal.*", Sender: memoryBroker},
    {Pattern: "audit.**", Sender: auditBroker},
})

// After: exact match only
selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
    {Topic: "internal/cache", Sender: memoryBroker},
    {Topic: "internal/events", Sender: memoryBroker},
    {Topic: "audit/log", Sender: auditBroker},
})
```

Wildcards can be added in v2 if needed.

---

## Summary of Recommended Changes

| Issue | Priority | Change |
|-------|----------|--------|
| #0 Duplicate brokers | High | Remove InMemoryBroker, keep ChannelBroker (rename to Broker) |
| #1 Concrete types | High | Return `*HTTPReceiver`, `*Broker` etc. |
| #2 Topic separator | High | Standardize on `/`, exact match only |
| #3 HTTP message clearing | High | Clear messages after Receive() returns |
| #4 HTTP ack handling | Medium | Add `WaitForAck` config: 201=immediate, 200=wait |
| #5 Subscription IDs | Medium | Use map with IDs instead of slice |
| #6 Feedback loop | Low | Already works: `publisher.Publish(ctx, router.Start(ctx, input))` |
| #7 Pattern matching | High | Remove for v1, exact match only |
