package channel_test

import (
	"testing"
	"time"

	"github.com/fxsml/gopipe/channel"
)

type testEvent struct {
	topic string
	data  int
}

func TestGroupBy_BasicGrouping(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour, // Long duration to test size-based batching
		},
	)

	// Send events for two topics
	go func() {
		defer close(in)
		in <- testEvent{"topic-a", 1}
		in <- testEvent{"topic-a", 2}
		in <- testEvent{"topic-b", 3}
		in <- testEvent{"topic-a", 4}
		in <- testEvent{"topic-b", 5}
	}()

	// Collect all batches
	results := make(map[string][]int)
	for batch := range batches {
		for _, item := range batch.Items {
			results[batch.Key] = append(results[batch.Key], item.data)
		}
	}

	// Verify grouping
	if len(results["topic-a"]) != 3 {
		t.Errorf("Expected 3 items for topic-a, got %d", len(results["topic-a"]))
	}
	if len(results["topic-b"]) != 2 {
		t.Errorf("Expected 2 items for topic-b, got %d", len(results["topic-b"]))
	}

	// Verify order within groups
	expectedA := []int{1, 2, 4}
	for i, v := range results["topic-a"] {
		if v != expectedA[i] {
			t.Errorf("topic-a[%d]: expected %d, got %d", i, expectedA[i], v)
		}
	}
}

func TestGroupBy_SizeBasedBatching(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 3, // Batch size of 3
			MaxDuration:  time.Hour,
		},
	)

	// Send events that should trigger size-based batching
	go func() {
		defer close(in)
		for i := 1; i <= 10; i++ {
			in <- testEvent{"topic-a", i}
		}
	}()

	// Collect batches
	var batchSizes []int
	for batch := range batches {
		batchSizes = append(batchSizes, len(batch.Items))
	}

	// Should have 3 batches of size 3, and 1 batch of size 1 (flush on close)
	if len(batchSizes) != 4 {
		t.Errorf("Expected 4 batches, got %d", len(batchSizes))
	}

	for i := 0; i < 3; i++ {
		if batchSizes[i] != 3 {
			t.Errorf("Batch %d: expected size 3, got %d", i, batchSizes[i])
		}
	}

	if batchSizes[3] != 1 {
		t.Errorf("Final batch: expected size 1, got %d", batchSizes[3])
	}
}

func TestGroupBy_TimeBasedBatching(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 100,                    // Large batch size
			MaxDuration:  100 * time.Millisecond, // Short duration
		},
	)

	// Send a few events, then wait for time-based flush
	go func() {
		defer close(in)
		in <- testEvent{"topic-a", 1}
		in <- testEvent{"topic-a", 2}
		time.Sleep(150 * time.Millisecond) // Wait for time-based flush
		in <- testEvent{"topic-a", 3}
	}()

	// Collect batches with timestamps
	var batches_received []channel.Group[string, testEvent]
	for batch := range batches {
		batches_received = append(batches_received, batch)
	}

	// Should have at least 2 batches due to time-based flushing
	if len(batches_received) < 2 {
		t.Errorf("Expected at least 2 batches due to time flush, got %d", len(batches_received))
	}

	// First batch should have 2 items (flushed by time)
	if len(batches_received[0].Items) != 2 {
		t.Errorf("First batch: expected 2 items, got %d", len(batches_received[0].Items))
	}
}

func TestGroupBy_MultipleGroups(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 2, // Small batch size
			MaxDuration:  time.Hour,
		},
	)

	// Send events for multiple topics
	go func() {
		defer close(in)
		in <- testEvent{"topic-a", 1}
		in <- testEvent{"topic-b", 2}
		in <- testEvent{"topic-c", 3}
		in <- testEvent{"topic-a", 4} // Completes topic-a batch
		in <- testEvent{"topic-b", 5} // Completes topic-b batch
		in <- testEvent{"topic-c", 6} // Completes topic-c batch
	}()

	// Collect all batches
	batchesByKey := make(map[string]int)
	for batch := range batches {
		batchesByKey[batch.Key]++
	}

	// Each topic should have 1 batch (size-based flush)
	for _, topic := range []string{"topic-a", "topic-b", "topic-c"} {
		if batchesByKey[topic] != 1 {
			t.Errorf("Topic %s: expected 1 batch, got %d", topic, batchesByKey[topic])
		}
	}
}

func TestGroupBy_MaxConcurrentGroups(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize:        10,
			MaxDuration:         time.Hour,
			MaxConcurrentGroups: 2, // Limit to 2 concurrent groups
		},
	)

	// Send events for 3 topics (exceeding the limit)
	go func() {
		defer close(in)
		in <- testEvent{"topic-a", 1}
		in <- testEvent{"topic-b", 2}
		in <- testEvent{"topic-c", 3} // Should evict topic-a
		in <- testEvent{"topic-a", 4} // New batch for topic-a
	}()

	// Collect all batches
	var allBatches []channel.Group[string, testEvent]
	for batch := range batches {
		allBatches = append(allBatches, batch)
	}

	// Should have at least 3 batches:
	// 1. topic-a evicted when topic-c arrives
	// 2. topic-b flushed on close
	// 3. topic-c flushed on close
	// 4. topic-a (new) flushed on close
	if len(allBatches) < 3 {
		t.Errorf("Expected at least 3 batches, got %d", len(allBatches))
	}

	// Verify topic-a was evicted and recreated
	topicABatches := 0
	for _, batch := range allBatches {
		if batch.Key == "topic-a" {
			topicABatches++
		}
	}

	if topicABatches != 2 {
		t.Errorf("Expected 2 batches for topic-a (evicted + recreated), got %d", topicABatches)
	}
}

func TestGroupBy_FlushOnClose(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
		},
	)

	go func() {
		defer close(in)
		in <- testEvent{"topic-a", 1}
		in <- testEvent{"topic-a", 2}
	}()

	var count int
	for batch := range batches {
		count += len(batch.Items)
	}

	if count != 2 {
		t.Errorf("Expected 2 items flushed on close, got %d", count)
	}
}

func TestGroupBy_EmptyChannel(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
		},
	)

	close(in) // Close immediately

	// Should produce no batches
	for range batches {
		t.Error("Expected no batches from empty channel")
	}
}

func TestGroupBy_SingleItem(t *testing.T) {
	in := make(chan testEvent)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
		},
	)

	go func() {
		defer close(in)
		in <- testEvent{"topic-a", 1}
	}()

	var count int
	for batch := range batches {
		count += len(batch.Items)
	}

	if count != 1 {
		t.Errorf("Expected 1 item, got %d", count)
	}
}

func TestGroupBy_IntegerKeys(t *testing.T) {
	type numberEvent struct {
		partition int
		value     string
	}

	in := make(chan numberEvent)
	batches := channel.GroupBy(
		in,
		func(e numberEvent) int { return e.partition },
		channel.GroupByConfig{
			MaxBatchSize: 2,
			MaxDuration:  time.Hour,
		},
	)

	go func() {
		defer close(in)
		in <- numberEvent{1, "a"}
		in <- numberEvent{2, "b"}
		in <- numberEvent{1, "c"} // Completes partition 1
		in <- numberEvent{2, "d"} // Completes partition 2
	}()

	batchCount := make(map[int]int)
	for batch := range batches {
		batchCount[batch.Key]++
	}

	if batchCount[1] != 1 || batchCount[2] != 1 {
		t.Errorf("Expected 1 batch per partition, got %v", batchCount)
	}
}

func TestGroupBy_ConcurrentWrites(t *testing.T) {
	in := make(chan testEvent, 100)
	batches := channel.GroupBy(
		in,
		func(e testEvent) string { return e.topic },
		channel.GroupByConfig{
			MaxBatchSize: 10,
			MaxDuration:  time.Hour,
		},
	)

	// Simulate concurrent writers
	go func() {
		for i := 0; i < 50; i++ {
			in <- testEvent{"topic-a", i}
		}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			in <- testEvent{"topic-b", i}
		}
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(in)
	}()

	// Count total items
	totalItems := 0
	for batch := range batches {
		totalItems += len(batch.Items)
	}

	if totalItems != 100 {
		t.Errorf("Expected 100 total items, got %d", totalItems)
	}
}

// BenchmarkGroupBy_ManyGroups benchmarks performance with many concurrent groups
func BenchmarkGroupBy_ManyGroups(b *testing.B) {
	for i := 0; i < b.N; i++ {
		in := make(chan testEvent, 1000)
		batches := channel.GroupBy(
			in,
			func(e testEvent) string { return e.topic },
			channel.GroupByConfig{
				MaxBatchSize: 100,
				MaxDuration:  time.Hour,
			},
		)

		go func() {
			for j := 0; j < 1000; j++ {
				in <- testEvent{topic: string(rune('A' + (j % 26))), data: j}
			}
			close(in)
		}()

		for range batches {
			// Drain batches
		}
	}
}

// BenchmarkGroupBy_FewGroups benchmarks performance with few groups
func BenchmarkGroupBy_FewGroups(b *testing.B) {
	for i := 0; i < b.N; i++ {
		in := make(chan testEvent, 1000)
		batches := channel.GroupBy(
			in,
			func(e testEvent) string { return e.topic },
			channel.GroupByConfig{
				MaxBatchSize: 100,
				MaxDuration:  time.Hour,
			},
		)

		go func() {
			for j := 0; j < 1000; j++ {
				in <- testEvent{topic: string(rune('A' + (j % 3))), data: j}
			}
			close(in)
		}()

		for range batches {
			// Drain batches
		}
	}
}
