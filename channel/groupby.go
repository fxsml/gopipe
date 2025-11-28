package channel

import (
	"sync"
	"time"
)

// Group represents a batch of items with the same key.
type Group[K comparable, V any] struct {
	Key   K
	Items []V
}

// GroupByConfig configures the GroupBy aggregator.
type GroupByConfig struct {
	// MaxConcurrentGroups limits the number of active groups.
	// When exceeded, new groups will cause the oldest group to be flushed.
	// Zero means no limit.
	MaxConcurrentGroups int

	// FlushOnClose determines whether to flush partial batches when the input channel closes.
	// Default is true.
	FlushOnClose bool
}

// GroupByOption configures GroupBy behavior.
type GroupByOption func(*GroupByConfig)

// WithMaxConcurrentGroups limits the number of active groups.
func WithMaxConcurrentGroups(max int) GroupByOption {
	return func(c *GroupByConfig) {
		c.MaxConcurrentGroups = max
	}
}

// WithFlushOnClose controls whether partial batches are flushed when input closes.
func WithFlushOnClose(flush bool) GroupByOption {
	return func(c *GroupByConfig) {
		c.FlushOnClose = flush
	}
}

// GroupBy groups items from the input channel by key and emits batches.
// Each group accumulates items until maxBatchSize is reached or maxDuration elapses.
//
// The keyFunc extracts the grouping key from each item.
// Items with the same key are batched together.
//
// Batches are emitted when:
//   - A group reaches maxBatchSize items
//   - maxDuration elapses since the first item in the group
//   - The input channel closes (if FlushOnClose is true, default)
//   - The context is canceled
//
// Example - Topic-based routing:
//
//	type Event struct {
//	    Topic string
//	    Data  any
//	}
//
//	batches := channel.GroupBy(
//	    events,
//	    func(e Event) string { return e.Topic },
//	    100,              // max 100 events per topic batch
//	    5*time.Second,    // or flush after 5 seconds
//	)
//
//	for batch := range batches {
//	    publisher.Publish(batch.Key, batch.Items) // Publish to topic
//	}
//
// Example - Multi-tenant processing:
//
//	batches := channel.GroupBy(
//	    requests,
//	    func(r Request) string { return r.TenantID },
//	    50,
//	    time.Second,
//	    channel.WithMaxConcurrentGroups(1000),
//	)
func GroupBy[V any, K comparable](
	in <-chan V,
	keyFunc func(V) K,
	maxBatchSize int,
	maxDuration time.Duration,
	opts ...GroupByOption,
) <-chan Group[K, V] {
	config := GroupByConfig{
		FlushOnClose: true,
	}
	for _, opt := range opts {
		opt(&config)
	}

	out := make(chan Group[K, V])

	go func() {
		defer close(out)
		groupBy(in, out, keyFunc, maxBatchSize, maxDuration, config)
	}()

	return out
}

// groupAccumulator manages batching for a single group.
type groupAccumulator[K comparable, V any] struct {
	key          K
	items        []V
	maxBatchSize int
	firstItemAt  time.Time
}

// newGroupAccumulator creates a new accumulator for a group.
func newGroupAccumulator[K comparable, V any](
	key K,
	maxBatchSize int,
) *groupAccumulator[K, V] {
	return &groupAccumulator[K, V]{
		key:          key,
		items:        make([]V, 0, maxBatchSize),
		maxBatchSize: maxBatchSize,
	}
}

// add adds an item to the accumulator and returns true if batch should be emitted.
func (a *groupAccumulator[K, V]) add(item V) bool {
	if len(a.items) == 0 {
		a.firstItemAt = time.Now()
	}
	a.items = append(a.items, item)
	return len(a.items) >= a.maxBatchSize
}

// flush returns the accumulated items and resets the accumulator.
func (a *groupAccumulator[K, V]) flush() []V {
	items := a.items
	a.items = make([]V, 0, a.maxBatchSize)
	return items
}

// groupBy implements the core grouping logic.
func groupBy[V any, K comparable](
	in <-chan V,
	out chan<- Group[K, V],
	keyFunc func(V) K,
	maxBatchSize int,
	maxDuration time.Duration,
	config GroupByConfig,
) {
	groups := make(map[K]*groupAccumulator[K, V])
	var mu sync.Mutex

	// Track insertion order for LRU eviction
	var groupOrder []K

	// emit sends a batch to the output channel.
	emit := func(key K, items []V) {
		if len(items) > 0 {
			out <- Group[K, V]{Key: key, Items: items}
		}
	}

	// evictOldest flushes and removes the oldest group.
	evictOldest := func() {
		if len(groupOrder) == 0 {
			return
		}
		oldestKey := groupOrder[0]
		groupOrder = groupOrder[1:]

		if acc, exists := groups[oldestKey]; exists {
			emit(oldestKey, acc.items)
			delete(groups, oldestKey)
		}
	}

	// getOrCreateGroup returns the accumulator for a key, creating if needed.
	getOrCreateGroup := func(key K) *groupAccumulator[K, V] {
		if acc, exists := groups[key]; exists {
			return acc
		}

		// Check if we need to evict to make room
		if config.MaxConcurrentGroups > 0 && len(groups) >= config.MaxConcurrentGroups {
			evictOldest()
		}

		acc := newGroupAccumulator[K, V](key, maxBatchSize)
		groups[key] = acc
		groupOrder = append(groupOrder, key)
		return acc
	}

	// flushAll flushes all groups and cleans up.
	flushAll := func() {
		for key, acc := range groups {
			if config.FlushOnClose {
				emit(key, acc.items)
			}
		}
		groups = make(map[K]*groupAccumulator[K, V])
		groupOrder = nil
	}

	// checkExpiredGroups flushes groups that have exceeded maxDuration.
	checkExpiredGroups := func() {
		now := time.Now()
		for key, acc := range groups {
			if len(acc.items) > 0 && now.Sub(acc.firstItemAt) >= maxDuration {
				items := acc.flush()
				emit(key, items)
			}
		}
	}

	// Start a goroutine to periodically check for expired groups
	ticker := time.NewTicker(maxDuration / 10)
	defer ticker.Stop()

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				checkExpiredGroups()
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	// Main processing loop
	for item := range in {
		mu.Lock()
		key := keyFunc(item)
		acc := getOrCreateGroup(key)

		if acc.add(item) {
			// Batch is full, flush it
			items := acc.flush()
			mu.Unlock()
			emit(key, items)
		} else {
			mu.Unlock()
		}
	}

	mu.Lock()
	flushAll()
	mu.Unlock()
}
