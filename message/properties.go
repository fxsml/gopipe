package message

import (
	"maps"
	"sync"
	"time"
)

// Reserved property keys for gopipe internal use.
// User-defined properties should NOT use the "gopipe." prefix.
const (
	// PropID is the unique message identifier.
	PropID = "gopipe.message.id"

	// PropCorrelationID is used to correlate related messages across services.
	PropCorrelationID = "gopipe.message.correlation_id"

	// PropCreatedAt stores when the message was created.
	PropCreatedAt = "gopipe.message.created_at"

	// PropRetryCount tracks how many times the message has been retried.
	PropRetryCount = "gopipe.message.retry_count"

	// PropDeadline stores the message processing deadline.
	PropDeadline = "gopipe.message.deadline"
)

// Properties provides thread-safe access to message properties.
// Reserved properties (gopipe.* keys) can only be set via functional options during message creation.
// Custom properties can be freely modified using Get/Set/Delete/Range methods.
type Properties struct {
	mu sync.RWMutex
	m  map[string]any
}

func NewProperties(p map[string]any) *Properties {
	props := &Properties{
		m: make(map[string]any),
	}
	maps.Copy(props.m, p)
	return props
}

// Get retrieves a value from the properties. Thread-safe for concurrent reads.
func (p *Properties) Get(key string) (any, bool) {
	if p == nil {
		return nil, false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	val, ok := p.m[key]
	return val, ok
}

// Set stores a key-value pair in the properties. Thread-safe for concurrent writes.
func (p *Properties) Set(key string, value any) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[key] = value
}

// Delete removes a key from the properties. Thread-safe for concurrent writes.
func (p *Properties) Delete(key string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, key)
}

// Range iterates over all key-value pairs in the properties.
// The iteration stops if f returns false. Thread-safe for concurrent reads.
func (p *Properties) Range(f func(key string, value any) bool) {
	if p == nil {
		return
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for k, v := range p.m {
		if !f(k, v) {
			break
		}
	}
}

// ID returns the message ID.
func (p *Properties) ID() string {
	if p == nil {
		return ""
	}
	if v, ok := p.Get(PropID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// CorrelationID returns the correlation ID.
func (p *Properties) CorrelationID() string {
	if p == nil {
		return ""
	}
	if v, ok := p.Get(PropCorrelationID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// CreatedAt returns when the message was created.
func (p *Properties) CreatedAt() time.Time {
	if p == nil {
		return time.Time{}
	}
	if v, ok := p.Get(PropCreatedAt); ok {
		if t, ok := v.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// RetryCount returns the number of times the message has been retried.
func (p *Properties) RetryCount() int {
	if p == nil {
		return 0
	}
	if v, ok := p.Get(PropRetryCount); ok {
		if count, ok := v.(int); ok {
			return count
		}
	}
	return 0
}

// IncrementRetryCount atomically increments and returns the new retry count.
// This is the only mutation allowed for the RetryCount reserved property after message creation.
// Thread-safe for concurrent access.
func (p *Properties) IncrementRetryCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// Read current count
	count := 0
	if v, ok := p.m[PropRetryCount]; ok {
		if c, ok := v.(int); ok {
			count = c
		}
	}

	// Increment and store
	count++
	p.m[PropRetryCount] = count
	return count
}
