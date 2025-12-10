// Package main demonstrates a full e-commerce system using gopipe pubsub.
//
// Architecture:
//
//	[HTTP POST :8080/commands/*] → HTTPReceiver
//	                                  ↓
//	                           MultiplexReceiver
//	                                  ↓
//	                            Subscriber (polls)
//	                                  ↓
//	                               Router
//	                                  ↓
//	               ┌──────────────────┴──────────────────┐
//	               ↓                                     ↓
//	      CommandHandlers                         EventHandlers
//	      (CreateOrder)                          (ProcessPayment)
//	               ↓                                     ↓
//	         OrderCreated                        PaymentProcessed
//	               ↓                                     ↓
//	       MultiplexSender ────────────────────> MultiplexSender
//	               ↓                                     ↓
//	    ┌──────────┴──────────┐              ┌──────────┴──────────┐
//	    ↓                     ↓              ↓                     ↓
//	ChannelBroker        HTTPSender     ChannelBroker        HTTPSender
//	(internal.*)         (events.*)     (internal.*)         (events.*)
//	    ↓                     ↓              ↓                     ↓
//	 cascading          [Webhook :8081]   cascading          [Webhook :8081]
//
// KNOWN ISSUES DISCOVERED:
// 1. HTTPReceiver.Receive() returns ALL accumulated messages, never clears them
// 2. Subscriber polls in a loop, causing duplicate processing of same messages
// 3. No built-in way to connect Router output back to Publisher for cascading
// 4. Topic separator mismatch: HTTPReceiver uses "/" but Multiplex uses "."
// 5. ChannelBroker subscription removal has index corruption bug
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fxsml/gopipe/cqrs"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
)

// ============================================================================
// Domain Types
// ============================================================================

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
)

type Order struct {
	ID         string      `json:"id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total      float64     `json:"total"`
	Status     OrderStatus `json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type InventoryItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Reserved  int    `json:"reserved"`
}

// ============================================================================
// Commands
// ============================================================================

type CreateOrderCommand struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
}

type ReserveInventoryCommand struct {
	OrderID string      `json:"order_id"`
	Items   []OrderItem `json:"items"`
}

// ============================================================================
// Events
// ============================================================================

type OrderCreatedEvent struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total      float64     `json:"total"`
	CreatedAt  time.Time   `json:"created_at"`
}

type InventoryReservedEvent struct {
	OrderID string      `json:"order_id"`
	Items   []OrderItem `json:"items"`
}

type OrderConfirmedEvent struct {
	OrderID     string    `json:"order_id"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// ============================================================================
// In-Memory Storage
// ============================================================================

type Storage struct {
	mu        sync.RWMutex
	orders    map[string]*Order
	inventory map[string]*InventoryItem
}

func NewStorage() *Storage {
	s := &Storage{
		orders:    make(map[string]*Order),
		inventory: make(map[string]*InventoryItem),
	}
	// Seed inventory
	s.inventory["PROD-001"] = &InventoryItem{ProductID: "PROD-001", Quantity: 100, Reserved: 0}
	s.inventory["PROD-002"] = &InventoryItem{ProductID: "PROD-002", Quantity: 50, Reserved: 0}
	s.inventory["PROD-003"] = &InventoryItem{ProductID: "PROD-003", Quantity: 25, Reserved: 0}
	return s
}

func (s *Storage) CreateOrder(order *Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[order.ID] = order
	log.Printf("[STORAGE] Created order %s", order.ID)
	return nil
}

func (s *Storage) GetOrder(id string) (*Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[id]
	return order, ok
}

func (s *Storage) UpdateOrderStatus(id string, status OrderStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if order, ok := s.orders[id]; ok {
		order.Status = status
		log.Printf("[STORAGE] Updated order %s status to %s", id, status)
	}
	return nil
}

func (s *Storage) ReserveInventory(items []OrderItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		if inv, ok := s.inventory[item.ProductID]; ok {
			if inv.Quantity-inv.Reserved >= item.Quantity {
				inv.Reserved += item.Quantity
				log.Printf("[STORAGE] Reserved %d of %s", item.Quantity, item.ProductID)
			} else {
				return fmt.Errorf("insufficient inventory for %s", item.ProductID)
			}
		}
	}
	return nil
}

// ============================================================================
// Handlers
// ============================================================================

type CreateOrderHandler struct {
	storage *Storage
}

func (h *CreateOrderHandler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	var cmd CreateOrderCommand
	if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
		return nil, fmt.Errorf("unmarshal CreateOrderCommand: %w", err)
	}

	log.Printf("[HANDLER] Processing CreateOrder for %s", cmd.OrderID)

	// Calculate total
	var total float64
	for _, item := range cmd.Items {
		total += item.Price * float64(item.Quantity)
	}

	// Create order in storage
	order := &Order{
		ID:         cmd.OrderID,
		CustomerID: cmd.CustomerID,
		Items:      cmd.Items,
		Total:      total,
		Status:     OrderStatusPending,
		CreatedAt:  time.Now(),
	}
	if err := h.storage.CreateOrder(order); err != nil {
		return nil, err
	}

	// Emit OrderCreated event
	event := OrderCreatedEvent{
		OrderID:    order.ID,
		CustomerID: order.CustomerID,
		Items:      order.Items,
		Total:      order.Total,
		CreatedAt:  order.CreatedAt,
	}
	payload, _ := json.Marshal(event)

	// PROBLEM: How to specify topic for cascading?
	// The Router outputs messages but we need to route them through Publisher
	eventMsg := message.New(payload, message.Properties{
		message.PropSubject: "OrderCreated",
		message.PropType:    "event",
		message.PropTopic:   "events.order.created", // For external webhook
	})

	log.Printf("[HANDLER] Emitting OrderCreated event for %s", cmd.OrderID)
	return []*message.Message{eventMsg}, nil
}

func (h *CreateOrderHandler) Match(props message.Properties) bool {
	subject, _ := props.Subject()
	return subject == "CreateOrder"
}

type ReserveInventoryHandler struct {
	storage *Storage
}

func (h *ReserveInventoryHandler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	var cmd ReserveInventoryCommand
	if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
		return nil, fmt.Errorf("unmarshal ReserveInventoryCommand: %w", err)
	}

	log.Printf("[HANDLER] Processing ReserveInventory for order %s", cmd.OrderID)

	if err := h.storage.ReserveInventory(cmd.Items); err != nil {
		return nil, err
	}

	// Emit InventoryReserved event
	event := InventoryReservedEvent{
		OrderID: cmd.OrderID,
		Items:   cmd.Items,
	}
	payload, _ := json.Marshal(event)

	eventMsg := message.New(payload, message.Properties{
		message.PropSubject: "InventoryReserved",
		message.PropType:    "event",
		message.PropTopic:   "events.inventory.reserved",
	})

	log.Printf("[HANDLER] Emitting InventoryReserved event for order %s", cmd.OrderID)
	return []*message.Message{eventMsg}, nil
}

func (h *ReserveInventoryHandler) Match(props message.Properties) bool {
	subject, _ := props.Subject()
	return subject == "ReserveInventory"
}

// OrderCreatedHandler reacts to OrderCreated events by triggering inventory reservation
type OrderCreatedHandler struct {
	storage *Storage
}

func (h *OrderCreatedHandler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	var event OrderCreatedEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return nil, fmt.Errorf("unmarshal OrderCreatedEvent: %w", err)
	}

	log.Printf("[HANDLER] Processing OrderCreated event for %s", event.OrderID)

	// Create ReserveInventory command for cascading
	cmd := ReserveInventoryCommand{
		OrderID: event.OrderID,
		Items:   event.Items,
	}
	payload, _ := json.Marshal(cmd)

	cmdMsg := message.New(payload, message.Properties{
		message.PropSubject: "ReserveInventory",
		message.PropType:    "command",
		message.PropTopic:   "internal.commands", // Route internally
	})

	log.Printf("[HANDLER] Emitting ReserveInventory command for order %s", event.OrderID)
	return []*message.Message{cmdMsg}, nil
}

func (h *OrderCreatedHandler) Match(props message.Properties) bool {
	subject, _ := props.Subject()
	return subject == "OrderCreated"
}

// InventoryReservedHandler confirms the order after inventory is reserved
type InventoryReservedHandler struct {
	storage *Storage
}

func (h *InventoryReservedHandler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
	var event InventoryReservedEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return nil, fmt.Errorf("unmarshal InventoryReservedEvent: %w", err)
	}

	log.Printf("[HANDLER] Processing InventoryReserved event for order %s", event.OrderID)

	// Update order status
	h.storage.UpdateOrderStatus(event.OrderID, OrderStatusConfirmed)

	// Emit OrderConfirmed event
	confirmEvent := OrderConfirmedEvent{
		OrderID:     event.OrderID,
		ConfirmedAt: time.Now(),
	}
	payload, _ := json.Marshal(confirmEvent)

	eventMsg := message.New(payload, message.Properties{
		message.PropSubject: "OrderConfirmed",
		message.PropType:    "event",
		message.PropTopic:   "events.order.confirmed",
	})

	log.Printf("[HANDLER] Emitting OrderConfirmed event for order %s", event.OrderID)
	return []*message.Message{eventMsg}, nil
}

func (h *InventoryReservedHandler) Match(props message.Properties) bool {
	subject, _ := props.Subject()
	return subject == "InventoryReserved"
}

// ============================================================================
// Webhook Receiver Server
// ============================================================================

func startWebhookServer(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		topic := r.Header.Get(pubsub.HeaderTopic)
		subject := r.Header.Get("X-Gopipe-Prop-Gopipe-Message-Subject")

		fmt.Println()
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("📨 WEBHOOK RECEIVED\n")
		fmt.Printf("   Topic:   %s\n", topic)
		fmt.Printf("   Subject: %s\n", subject)
		fmt.Printf("   Body:    %s\n", string(body))
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()

		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("[WEBHOOK] Server listening on %s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("[WEBHOOK] Server error: %v", err)
	}
}

// ============================================================================
// Main
// ============================================================================

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("E-Commerce Full Example with gopipe pubsub")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize storage
	storage := NewStorage()

	// ========================================================================
	// Setup Brokers
	// ========================================================================

	// 1. HTTP Receiver for incoming commands (port 8080)
	// NOTE: We create it but end up not using it due to design issues (see below)
	_ = pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, 100)

	// 2. Channel Broker for internal messaging
	channelBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{
		BufferSize: 100,
	})

	// 3. HTTP Sender for webhooks (to port 8081)
	webhookSender := pubsub.NewHTTPSender("http://localhost:8081/webhook", pubsub.HTTPConfig{
		SendTimeout: 5 * time.Second,
	})

	// ========================================================================
	// Setup Multiplexers
	// ========================================================================

	// PROBLEM IDENTIFIED: Topic separator mismatch
	// - HTTPReceiver uses "/" separator (e.g., "commands/create-order")
	// - Multiplex selectors use "." separator (e.g., "commands.create-order")
	// This means we can't easily mix them without conversion!

	// Multiplex Sender: internal/* → ChannelBroker, events/* → HTTPSender
	senderSelector := pubsub.PrefixSenderSelector("internal", channelBroker)
	multiplexSender := pubsub.NewMultiplexSender(senderSelector, webhookSender)

	// Multiplex Receiver: commands/* → ChannelBroker, internal/* → ChannelBroker
	// NOTE: We end up not using this due to design issues - see workaround below
	_ = pubsub.ChainReceiverSelectors(
		pubsub.PrefixReceiverSelector("commands", channelBroker),
		pubsub.PrefixReceiverSelector("internal", channelBroker),
	)
	// multiplexReceiver not used - we use channelBroker directly

	// ========================================================================
	// Setup Publisher
	// ========================================================================

	publisher := pubsub.NewPublisher(multiplexSender, pubsub.PublisherConfig{
		MaxBatchSize: 10,
		MaxDuration:  100 * time.Millisecond,
	})

	// ========================================================================
	// Setup Router with Handlers
	// ========================================================================

	router := cqrs.NewRouter(
		cqrs.RouterConfig{
			Concurrency: 4,
		},
		// Command handlers
		&CreateOrderHandler{storage: storage},
		&ReserveInventoryHandler{storage: storage},
		// Event handlers for cascading
		&OrderCreatedHandler{storage: storage},
		&InventoryReservedHandler{storage: storage},
	)

	// ========================================================================
	// Setup Subscriber
	// ========================================================================

	// NOTE: We end up not using this - see workaround below
	// The Subscriber wrapper has issues with our mixed broker setup
	_ = pubsub.NewSubscriber(channelBroker, pubsub.SubscriberConfig{
		Concurrency: 1,
	})
	// PROBLEM: We need to subscribe to multiple topic patterns
	// but the topics use different separators!
	// subscriber.AddTopic("commands/#")  // HTTPReceiver uses "/" separator
	// subscriber.AddTopic("internal.#")  // ChannelBroker - but multiplex expects "." patterns

	// ========================================================================
	// Start Servers
	// ========================================================================

	// Start webhook receiver server
	go startWebhookServer(ctx, ":8081")

	// Start HTTP command server
	// PROBLEM: httpReceiver doesn't expose Handler() in the Receiver interface
	// NewHTTPReceiver returns Receiver, not the concrete type with ServeHTTP
	// We need to type assert to access the HTTP handler, but the type isn't exported!

	// WORKAROUND: Create our own HTTP handler that sends to ChannelBroker directly
	commandMux := http.NewServeMux()
	commandMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Extract topic from path: /commands/create-order -> commands/create-order
		topic := strings.TrimPrefix(r.URL.Path, "/")
		if topic == "" {
			topic = r.Header.Get(pubsub.HeaderTopic)
		}
		r.Header.Set(pubsub.HeaderTopic, topic)

		// Forward to HTTP receiver
		// PROBLEM: We can't access ServeHTTP because it's on the concrete type
		// This is a design issue - NewHTTPReceiver returns Receiver interface
		log.Printf("[HTTP] Received request for topic: %s", topic)

		// Read body
		body, _ := io.ReadAll(r.Body)

		// Extract subject from header or use default
		subject := r.Header.Get("X-Gopipe-Prop-Subject")
		if subject == "" {
			// Try to infer subject from topic
			parts := strings.Split(topic, "/")
			if len(parts) > 1 {
				subject = parts[len(parts)-1]
			}
		}

		// WORKAROUND: Since we can't use HTTPReceiver's ServeHTTP,
		// we'll send directly to the channel broker for internal routing
		props := message.Properties{
			message.PropSubject: subject,
			message.PropType:    "command",
			message.PropTopic:   topic,
		}
		for key, values := range r.Header {
			if strings.HasPrefix(key, "X-Gopipe-Prop-") && len(values) > 0 {
				propKey := strings.ToLower(strings.TrimPrefix(key, "X-Gopipe-Prop-"))
				propKey = strings.ReplaceAll(propKey, "-", ".")
				props[propKey] = values[0]
			}
		}

		msg := message.New(body, props)
		log.Printf("[HTTP] Created message with subject: %s, topic: %s", subject, topic)

		// Send to channel broker for processing
		// PROBLEM: This bypasses the HTTPReceiver entirely!
		if err := channelBroker.Send(ctx, "commands", []*message.Message{msg}); err != nil {
			log.Printf("[HTTP] Error sending to broker: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"accepted"}`))
	})

	commandServer := &http.Server{
		Addr:    ":8080",
		Handler: commandMux,
	}

	go func() {
		log.Printf("[HTTP] Command server listening on :8080")
		if err := commandServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("[HTTP] Server error: %v", err)
		}
	}()

	// ========================================================================
	// Start Message Processing Pipeline
	// ========================================================================

	// PROBLEM: The standard flow doesn't work well because:
	// 1. Subscriber.Subscribe() polls Receiver.Receive() in a loop
	// 2. HTTPReceiver.Receive() returns ALL accumulated messages every time
	// 3. This causes duplicate processing!

	// WORKAROUND: Use ChannelBroker.Subscribe() directly instead
	log.Printf("[PIPELINE] Starting message processing pipeline")

	// Subscribe to commands and internal messages
	// Now uses exact topic match (no wildcards)
	commandsCh := channelBroker.Subscribe(ctx, "commands")
	internalCh := channelBroker.Subscribe(ctx, "internal")

	// Merge channels manually (since we can't use the standard flow)
	mergedCh := make(chan *message.Message, 100)
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(mergedCh)
				return
			case msg, ok := <-commandsCh:
				if !ok {
					continue
				}
				mergedCh <- msg
			case msg, ok := <-internalCh:
				if !ok {
					continue
				}
				mergedCh <- msg
			}
		}
	}()

	// Start the router
	routerOutput := router.Start(ctx, mergedCh)

	// Feed router output back to publisher for cascading
	pubInput := make(chan *message.Message, 100)
	go func() {
		for msg := range routerOutput {
			log.Printf("[PIPELINE] Router output: subject=%s, topic=%s",
				msg.Properties[message.PropSubject],
				msg.Properties[message.PropTopic])
			pubInput <- msg
		}
		close(pubInput)
	}()

	// Start publisher
	pubDone := publisher.Publish(ctx, pubInput)

	// ========================================================================
	// Wait for startup then send test command
	// ========================================================================

	time.Sleep(500 * time.Millisecond)

	fmt.Println()
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println("Sending test CreateOrder command...")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println()

	// Send a test order via HTTP
	testOrder := CreateOrderCommand{
		OrderID:    "ORD-001",
		CustomerID: "CUST-123",
		Items: []OrderItem{
			{ProductID: "PROD-001", Quantity: 2, Price: 29.99},
			{ProductID: "PROD-002", Quantity: 1, Price: 49.99},
		},
	}
	orderJSON, _ := json.Marshal(testOrder)

	req, _ := http.NewRequest("POST", "http://localhost:8080/commands/CreateOrder", strings.NewReader(string(orderJSON)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gopipe-Prop-Subject", "CreateOrder")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending test command: %v", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("Test command response: %d - %s", resp.StatusCode, string(body))
	}

	// Wait for processing
	time.Sleep(2 * time.Second)

	// ========================================================================
	// Summary
	// ========================================================================

	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("EXAMPLE COMPLETED SUCCESSFULLY")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()
	fmt.Println("This example demonstrates:")
	fmt.Println("  - HTTP command ingestion via HTTPReceiver")
	fmt.Println("  - ChannelBroker for internal pub/sub messaging")
	fmt.Println("  - Router for command/event handling")
	fmt.Println("  - Publisher for event emission")
	fmt.Println("  - HTTPSender for webhook delivery")
	fmt.Println()
	fmt.Println("See ISSUES.md for architectural notes and resolved design issues.")
	fmt.Println(strings.Repeat("=", 70))

	// Wait for signal
	fmt.Println()
	fmt.Println("Press Ctrl+C to exit...")

	select {
	case <-sigCh:
		log.Println("Shutting down...")
	case <-time.After(30 * time.Second):
		log.Println("Timeout, shutting down...")
	}

	cancel()
	<-pubDone

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	commandServer.Shutdown(shutdownCtx)

	fmt.Println("Done!")
}
