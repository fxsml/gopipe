// Package main demonstrates the broker adapters for NATS, Kafka, and RabbitMQ.
//
// This example shows the Subscriber[*Message] pattern from ADR 0030,
// proving that broker-specific implementations provide more flexibility
// than the generic Sender/Receiver interfaces.
//
// Run with: docker-compose up -d && go run main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/examples/adapters/kafka"
	adapter_nats "github.com/fxsml/gopipe/examples/adapters/nats"
	"github.com/fxsml/gopipe/examples/adapters/rabbitmq"
	"github.com/fxsml/gopipe/message"
)

// Order represents a sample event type
type Order struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"` // "created", "updated", "deleted"
	Amount float64 `json:"amount"`
}

func main() {
	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Parse command line
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <nats|kafka|rabbitmq|all>")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	switch os.Args[1] {
	case "nats":
		runNATSDemo(ctx)
	case "kafka":
		runKafkaDemo(ctx)
	case "rabbitmq":
		runRabbitMQDemo(ctx)
	case "all":
		runAllDemo(ctx)
	default:
		fmt.Printf("Unknown broker: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runNATSDemo(ctx context.Context) {
	fmt.Println("\n=== NATS Demo ===")
	fmt.Println("Demonstrating: Subject wildcards, queue groups")

	// Create subscriber with wildcard subject
	sub := adapter_nats.NewSubscriber(adapter_nats.SubscriberConfig{
		URL:        "nats://localhost:4222",
		Subject:    "orders.>", // Wildcard: receives orders.created, orders.updated, etc.
		Queue:      "order-processors",
		BufferSize: 100,
	})

	// Create publisher
	pub := adapter_nats.NewPublisher(adapter_nats.PublisherConfig{
		URL: "nats://localhost:4222",
	})
	if err := pub.Connect(ctx); err != nil {
		slog.Error("Failed to connect publisher", "error", err)
		return
	}
	defer pub.Close()

	// Start subscriber
	msgs := sub.Subscribe(ctx)

	// Publish some test messages
	go func() {
		time.Sleep(time.Second) // Wait for subscriber to be ready

		orders := []Order{
			{ID: "1", Type: "created", Amount: 100.00},
			{ID: "2", Type: "updated", Amount: 150.00},
			{ID: "3", Type: "deleted", Amount: 0},
		}

		for _, order := range orders {
			data, _ := json.Marshal(order)
			msg := message.New(data, message.Attributes{
				"type": "order." + order.Type,
			})

			// Publish to subject like "orders.created"
			subject := "orders." + order.Type
			if err := pub.Publish(ctx, subject, msg); err != nil {
				slog.Error("Failed to publish", "error", err)
			} else {
				slog.Info("Published to NATS", "subject", subject, "order_id", order.ID)
			}
		}
	}()

	// Receive messages
	count := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			slog.Info("Received from NATS",
				"subject", msg.Attributes["nats.subject"],
				"data", string(msg.Data),
			)
			count++
			if count >= 3 {
				return
			}
		case <-timeout:
			slog.Warn("Timeout waiting for messages")
			return
		case <-ctx.Done():
			return
		}
	}
}

func runKafkaDemo(ctx context.Context) {
	fmt.Println("\n=== Kafka Demo ===")
	fmt.Println("Demonstrating: Consumer groups, partition-level ordering, explicit topics")

	// Create subscriber with consumer group
	sub := kafka.NewSubscriber(kafka.SubscriberConfig{
		Brokers:       []string{"localhost:9092"},
		Topics:        []string{"orders"}, // No wildcards - explicit topic
		ConsumerGroup: "order-processor-group",
		BufferSize:    100,
	})

	// Create publisher
	pub := kafka.NewPublisher(kafka.PublisherConfig{
		Brokers: []string{"localhost:9092"},
	})
	defer pub.Close()

	// Start subscriber
	msgs := sub.Subscribe(ctx)

	// Publish some test messages
	go func() {
		time.Sleep(2 * time.Second) // Kafka takes longer to start

		orders := []Order{
			{ID: "K1", Type: "created", Amount: 200.00},
			{ID: "K2", Type: "updated", Amount: 250.00},
			{ID: "K3", Type: "deleted", Amount: 0},
		}

		for _, order := range orders {
			data, _ := json.Marshal(order)
			msg := message.New(data, message.Attributes{
				"key":  order.ID, // Partition key - same ID goes to same partition
				"type": "order." + order.Type,
			})

			if err := pub.Publish(ctx, "orders", msg); err != nil {
				slog.Error("Failed to publish to Kafka", "error", err)
			} else {
				slog.Info("Published to Kafka", "topic", "orders", "key", order.ID)
			}
		}
	}()

	// Receive messages
	count := 0
	timeout := time.After(10 * time.Second)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			slog.Info("Received from Kafka",
				"topic", msg.Attributes["kafka.topic"],
				"partition", msg.Attributes["kafka.partition"],
				"offset", msg.Attributes["kafka.offset"],
				"key", msg.Attributes["kafka.key"],
			)
			// Acknowledge the message
			msg.Ack()
			count++
			if count >= 3 {
				return
			}
		case <-timeout:
			slog.Warn("Timeout waiting for Kafka messages")
			return
		case <-ctx.Done():
			return
		}
	}
}

func runRabbitMQDemo(ctx context.Context) {
	fmt.Println("\n=== RabbitMQ Demo ===")
	fmt.Println("Demonstrating: Exchange/queue/binding model, routing patterns")

	// Create subscriber with topic exchange and binding pattern
	sub := rabbitmq.NewSubscriber(rabbitmq.SubscriberConfig{
		URL:          "amqp://guest:guest@localhost:5672/",
		Exchange:     "orders",
		ExchangeType: "topic",                  // Topic exchange for pattern routing
		Queue:        "order-processor-queue",
		BindingKey:   "orders.#",              // Receive all orders.* messages
		Durable:      true,
	})

	// Create publisher
	pub := rabbitmq.NewPublisher(rabbitmq.PublisherConfig{
		URL:          "amqp://guest:guest@localhost:5672/",
		Exchange:     "orders",
		ExchangeType: "topic",
		Durable:      true,
	})
	if err := pub.Connect(ctx); err != nil {
		slog.Error("Failed to connect RabbitMQ publisher", "error", err)
		return
	}
	defer pub.Close()

	// Start subscriber
	msgs := sub.Subscribe(ctx)

	// Publish some test messages
	go func() {
		time.Sleep(time.Second) // Wait for queues to be ready

		orders := []Order{
			{ID: "R1", Type: "created", Amount: 300.00},
			{ID: "R2", Type: "updated", Amount: 350.00},
			{ID: "R3", Type: "deleted", Amount: 0},
		}

		for _, order := range orders {
			data, _ := json.Marshal(order)
			msg := message.New(data, message.Attributes{
				"type": "order." + order.Type,
			})

			// Routing key like "orders.created"
			routingKey := "orders." + order.Type
			if err := pub.Publish(ctx, routingKey, msg); err != nil {
				slog.Error("Failed to publish to RabbitMQ", "error", err)
			} else {
				slog.Info("Published to RabbitMQ", "routing_key", routingKey, "order_id", order.ID)
			}
		}
	}()

	// Receive messages
	count := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			slog.Info("Received from RabbitMQ",
				"routing_key", msg.Attributes["rabbitmq.routing_key"],
				"exchange", msg.Attributes["rabbitmq.exchange"],
				"delivery_tag", msg.Attributes["rabbitmq.delivery_tag"],
			)
			// Acknowledge the message
			msg.Ack()
			count++
			if count >= 3 {
				return
			}
		case <-timeout:
			slog.Warn("Timeout waiting for RabbitMQ messages")
			return
		case <-ctx.Done():
			return
		}
	}
}

func runAllDemo(ctx context.Context) {
	fmt.Println("\n=== All Brokers Demo ===")
	fmt.Println("Running all broker demos with channel.GroupBy for publishing")

	var wg sync.WaitGroup

	// Generate sample messages
	orders := make(chan *message.Message, 100)
	go func() {
		defer close(orders)
		for i := 1; i <= 9; i++ {
			orderType := []string{"created", "updated", "deleted"}[i%3]
			order := Order{
				ID:     fmt.Sprintf("ALL-%d", i),
				Type:   orderType,
				Amount: float64(i) * 100,
			}
			data, _ := json.Marshal(order)
			msg := message.New(data, message.Attributes{
				"destination": "orders." + orderType,
				"type":        "order." + orderType,
			})
			select {
			case orders <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Use channel.GroupBy to batch by destination
	groups := channel.GroupBy(orders, func(msg *message.Message) string {
		if dest, ok := msg.Attributes["destination"].(string); ok {
			return dest
		}
		return "default"
	}, channel.GroupByConfig{
		MaxBatchSize: 3,
		MaxDuration:  time.Second,
	})

	// Process groups
	wg.Add(1)
	go func() {
		defer wg.Done()
		for group := range groups {
			slog.Info("Grouped messages",
				"destination", group.Key,
				"count", len(group.Items),
			)
		}
	}()

	wg.Wait()
	fmt.Println("\n=== Demo Complete ===")
}
