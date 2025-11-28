package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// Event represents a domain event to be published to a topic.
type Event struct {
	Topic   string
	Payload any
}

// Publisher simulates a message broker that publishes events to topics in batches.
type Publisher struct {
	publishCount atomic.Int64
}

// PublishBatch publishes a batch of events to a topic.
// In a real implementation, this would send to Kafka, RabbitMQ, etc.
func (p *Publisher) PublishBatch(topic string, events []*message.Message[Event]) error {
	p.publishCount.Add(int64(len(events)))

	fmt.Printf("\n📤 Publishing batch to topic '%s' (%d events)\n", topic, len(events))
	for i, msg := range events {
		event := msg.Payload()
		msgID, _ := msg.Properties().Get(message.PropID)
		fmt.Printf("  [%d] ID=%v, Data=%v\n", i+1, msgID, event.Payload)

		// Acknowledge message after successful publish
		msg.Ack()
	}

	return nil
}

func main() {
	ctx := context.Background()
	publisher := &Publisher{}

	// Track acknowledgments
	var ackCount, nackCount atomic.Int64
	ack := func() { ackCount.Add(1) }
	nack := func(err error) {
		nackCount.Add(1)
		fmt.Printf("✗ Message nack: %v\n", err)
	}

	// Create a stream of events for various topics
	events := make(chan *message.Message[Event], 100)

	go func() {
		defer close(events)

		// Simulate events coming from different sources
		topicData := map[string][]any{
			"orders.created":   {"order-1", "order-2", "order-3", "order-4", "order-5"},
			"orders.updated":   {"order-6", "order-7"},
			"inventory.low":    {"sku-001", "sku-002", "sku-003"},
			"users.registered": {"user-a", "user-b", "user-c", "user-d"},
			"payments.success": {"pay-1", "pay-2", "pay-3", "pay-4", "pay-5", "pay-6"},
		}

		msgID := 0
		for topic, payloads := range topicData {
			for _, payload := range payloads {
				msgID++
				events <- message.New(
					Event{Topic: topic, Payload: payload},
					message.WithContext[Event](ctx),
					message.WithAcking[Event](ack, nack),
					message.WithID[Event](fmt.Sprintf("msg-%03d", msgID)),
					message.WithProperty[Event]("source", "event-stream"),
					message.WithCreatedAt[Event](time.Now()),
				)

				// Simulate realistic event arrival timing
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Group events by topic and batch them
	// Batches are emitted when:
	// - 5 events for the same topic are accumulated, OR
	// - 200ms have elapsed since the first event in the group
	batches := channel.GroupBy(
		events,
		func(msg *message.Message[Event]) string {
			return msg.Payload().Topic
		},
		5,               // Max batch size per topic
		200*time.Millisecond, // Max wait time
	)

	// Publish batches to their respective topics
	fmt.Println("🚀 Starting topic-based event router...")
	fmt.Println("   Batching by topic with maxSize=5, maxDuration=200ms")
	fmt.Println()

	startTime := time.Now()
	batchCount := 0

	for batch := range batches {
		batchCount++
		if err := publisher.PublishBatch(batch.Key, batch.Items); err != nil {
			fmt.Printf("❌ Failed to publish batch: %v\n", err)
			// Nack all messages in the batch
			for _, msg := range batch.Items {
				msg.Nack(err)
			}
		}
	}

	duration := time.Since(startTime)
	totalEvents := publisher.publishCount.Load()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("✅ Routing complete\n\n")
	fmt.Printf("Statistics:\n")
	fmt.Printf("  Total events:     %d\n", totalEvents)
	fmt.Printf("  Total batches:    %d\n", batchCount)
	fmt.Printf("  Avg batch size:   %.1f events/batch\n", float64(totalEvents)/float64(batchCount))
	fmt.Printf("  Acknowledged:     %d\n", ackCount.Load())
	fmt.Printf("  Rejected:         %d\n", nackCount.Load())
	fmt.Printf("  Duration:         %v\n", duration)
	fmt.Printf("  Throughput:       %.0f events/sec\n", float64(totalEvents)/(duration.Seconds()))
	fmt.Println(strings.Repeat("=", 60))
}
