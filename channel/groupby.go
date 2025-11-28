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
	// MaxBatchSize is the maximum number of items per batch for each group.
	// When a group reaches this size, it is immediately flushed.
	// Default is 100.
	MaxBatchSize int

	// MaxDuration is the maximum time to wait before flushing a group.
	// A group is flushed when MaxDuration elapses since the first item was added.
	// Default is 1 second.
	MaxDuration time.Duration

	// MaxConcurrentGroups limits the number of active groups.
	// When exceeded, new groups will cause the oldest group to be flushed (LRU eviction).
	// Default is 0 (no limit).
	MaxConcurrentGroups int
}

// applyDefaults returns a config with defaults applied for any zero-value fields.
func (c GroupByConfig) applyDefaults() GroupByConfig {
	if c.MaxBatchSize <= 0 {
		c.MaxBatchSize = 100
	}
	if c.MaxDuration <= 0 {
		c.MaxDuration = time.Second
	}
	return c
}

// GroupBy groups items from the input channel by key and emits batches.
// Each group accumulates items until MaxBatchSize is reached or MaxDuration elapses.
//
// The keyFunc extracts the grouping key from each item.
// Items with the same key are batched together.
//
// Batches are emitted when:
//   - A group reaches MaxBatchSize items
//   - MaxDuration elapses since the first item in the group
//   - The input channel closes (partial batches are flushed)
//
// Use GroupByConfig to configure batching behavior. Zero-value fields use defaults:
//   - MaxBatchSize: 100
//   - MaxDuration: 1 second
//   - MaxConcurrentGroups: 0 (unlimited)
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
//	    channel.GroupByConfig{
//	        MaxBatchSize: 100,
//	        MaxDuration:  5 * time.Second,
//	    },
//	)
//
//	for batch := range batches {
//	    publisher.Publish(batch.Key, batch.Items)
//	}
//
// Example - Multi-tenant processing with defaults:
//
//	batches := channel.GroupBy(
//	    requests,
//	    func(r Request) string { return r.TenantID },
//	    channel.GroupByConfig{
//	        MaxConcurrentGroups: 1000,
//	    },
//	)
//
// Example - Using all defaults:
//
//	batches := channel.GroupBy(
//	    events,
//	    func(e Event) string { return e.Topic },
//	    channel.GroupByConfig{},
//	)
func GroupBy[V any, K comparable](
	in <-chan V,
	keyFunc func(V) K,
	config GroupByConfig,
) <-chan Group[K, V] {
	config = config.applyDefaults()

	out := make(chan Group[K, V])

	go func() {
		defer close(out)
		groupBy(in, out, keyFunc, config)
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
	config GroupByConfig,
) *groupAccumulator[K, V] {
	return &groupAccumulator[K, V]{
		key:          key,
		items:        make([]V, 0, config.MaxBatchSize),
		maxBatchSize: config.MaxBatchSize,
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

		acc := newGroupAccumulator[K, V](key, config)
		groups[key] = acc
		groupOrder = append(groupOrder, key)
		return acc
	}

	// flushAll flushes all groups and cleans up.
	flushAll := func() {
		for key, acc := range groups {
			emit(key, acc.items)
		}
		groups = make(map[K]*groupAccumulator[K, V])
		groupOrder = nil
	}

	// checkExpiredGroups flushes groups that have exceeded MaxDuration.
	checkExpiredGroups := func() {
		now := time.Now()
		for key, acc := range groups {
			if len(acc.items) > 0 && now.Sub(acc.firstItemAt) >= config.MaxDuration {
				items := acc.flush()
				emit(key, items)
			}
		}
	}

	// Start a goroutine to periodically check for expired groups
	ticker := time.NewTicker(config.MaxDuration / 10)
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
