// Package examples demonstrates go-amqp integration with goengine.
//
// This example shows how to use Azure/go-amqp as a universal AMQP 1.0 adapter
// for Azure Service Bus, Event Hubs, Apache Qpid, and other AMQP 1.0 brokers.
//
// Key concepts:
// - Direct go-amqp for connection management
// - CloudEvents binary/structured content modes
// - Proper acknowledgment handling
// - goengine Subscriber/Publisher pattern integration
package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	// In real implementation:
	// "github.com/Azure/go-amqp"
)

// ============================================================================
// MOCK go-amqp TYPES (in real code, import github.com/Azure/go-amqp)
// ============================================================================

// ConnOptions configures AMQP connection
type ConnOptions struct {
	SASLType SASLType
}

// SASLType for authentication
type SASLType interface{}

// SASLTypePlain creates plain auth
func SASLTypePlain(username, password string) SASLType {
	return &plainAuth{username, password}
}

type plainAuth struct{ user, pass string }

// Conn represents an AMQP connection
type Conn struct {
	url    string
	closed bool
	mu     sync.Mutex
}

// Dial creates a new AMQP connection
func Dial(ctx context.Context, url string, opts *ConnOptions) (*Conn, error) {
	return &Conn{url: url}, nil
}

// NewSession creates a session on the connection
func (c *Conn) NewSession(ctx context.Context, opts *SessionOptions) (*Session, error) {
	return &Session{conn: c}, nil
}

// Close closes the connection
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// SessionOptions configures AMQP session
type SessionOptions struct{}

// Session represents an AMQP session
type Session struct {
	conn *Conn
}

// NewSender creates a message sender
func (s *Session) NewSender(ctx context.Context, target string, opts *SenderOptions) (*Sender, error) {
	return &Sender{session: s, target: target}, nil
}

// NewReceiver creates a message receiver
func (s *Session) NewReceiver(ctx context.Context, source string, opts *ReceiverOptions) (*Receiver, error) {
	return &Receiver{
		session: s,
		source:  source,
		msgChan: make(chan *AMQPMessage, 100),
	}, nil
}

// SenderOptions configures sender
type SenderOptions struct{}

// ReceiverOptions configures receiver
type ReceiverOptions struct {
	Credit uint32 // Prefetch count
}

// Sender sends AMQP messages
type Sender struct {
	session *Session
	target  string
}

// Send sends a message
func (s *Sender) Send(ctx context.Context, msg *AMQPMessage, opts *SendOptions) error {
	fmt.Printf("[AMQP] Sent to %s: %s\n", s.target, string(msg.Data[0]))
	return nil
}

// SendOptions configures send operation
type SendOptions struct{}

// Receiver receives AMQP messages
type Receiver struct {
	session *Session
	source  string
	msgChan chan *AMQPMessage
}

// Receive waits for a message
func (r *Receiver) Receive(ctx context.Context, opts *ReceiveOptions) (*AMQPMessage, error) {
	select {
	case msg := <-r.msgChan:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// AcceptMessage acknowledges successful processing
func (r *Receiver) AcceptMessage(ctx context.Context, msg *AMQPMessage) error {
	fmt.Printf("[AMQP] Accepted message: %s\n", msg.Properties.MessageID)
	return nil
}

// RejectMessage rejects with optional error
func (r *Receiver) RejectMessage(ctx context.Context, msg *AMQPMessage, e *Error) error {
	fmt.Printf("[AMQP] Rejected message: %s\n", msg.Properties.MessageID)
	return nil
}

// ReleaseMessage releases for redelivery
func (r *Receiver) ReleaseMessage(ctx context.Context, msg *AMQPMessage) error {
	fmt.Printf("[AMQP] Released message: %s\n", msg.Properties.MessageID)
	return nil
}

// ReceiveOptions configures receive operation
type ReceiveOptions struct{}

// Error for AMQP error conditions
type Error struct {
	Condition   string
	Description string
}

// AMQPMessage represents an AMQP message
type AMQPMessage struct {
	Data                  [][]byte
	Properties            *MessageProperties
	ApplicationProperties map[string]any
}

// MessageProperties holds standard AMQP properties
type MessageProperties struct {
	MessageID     string
	ContentType   string
	CorrelationID string
	Subject       string
}

// NewAMQPMessage creates a message with data
func NewAMQPMessage(data []byte) *AMQPMessage {
	return &AMQPMessage{
		Data:                  [][]byte{data},
		Properties:            &MessageProperties{},
		ApplicationProperties: make(map[string]any),
	}
}

// ============================================================================
// CLOUDEVENTS INTEGRATION
// ============================================================================

// CloudEvent represents a CloudEvents message (simplified)
type CloudEvent struct {
	SpecVersion     string         `json:"specversion"`
	ID              string         `json:"id"`
	Source          string         `json:"source"`
	Type            string         `json:"type"`
	Subject         string         `json:"subject,omitempty"`
	Time            string         `json:"time,omitempty"`
	DataContentType string         `json:"datacontenttype,omitempty"`
	Data            any            `json:"data,omitempty"`
	Extensions      map[string]any `json:"-"`
}

// CloudEventsCodec handles CE ↔ AMQP conversion
type CloudEventsCodec struct{}

// ToAMQPBinary converts CloudEvent to AMQP binary mode
// CE attributes go to application-properties with ce_ prefix
func (c *CloudEventsCodec) ToAMQPBinary(event *CloudEvent) (*AMQPMessage, error) {
	// Serialize data
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	msg := NewAMQPMessage(dataBytes)

	// Map CE attributes to AMQP properties
	msg.Properties.MessageID = event.ID
	msg.Properties.ContentType = event.DataContentType
	if msg.Properties.ContentType == "" {
		msg.Properties.ContentType = "application/json"
	}
	msg.Properties.Subject = event.Subject

	// Map CE attributes to application-properties with ce_ prefix
	msg.ApplicationProperties["ce_specversion"] = event.SpecVersion
	msg.ApplicationProperties["ce_id"] = event.ID
	msg.ApplicationProperties["ce_source"] = event.Source
	msg.ApplicationProperties["ce_type"] = event.Type

	if event.Subject != "" {
		msg.ApplicationProperties["ce_subject"] = event.Subject
	}
	if event.Time != "" {
		msg.ApplicationProperties["ce_time"] = event.Time
	}
	if event.DataContentType != "" {
		msg.ApplicationProperties["ce_datacontenttype"] = event.DataContentType
	}

	// Map extensions
	for k, v := range event.Extensions {
		msg.ApplicationProperties["ce_"+k] = v
	}

	return msg, nil
}

// ToAMQPStructured converts CloudEvent to AMQP structured mode
// Entire CE as JSON in message body
func (c *CloudEventsCodec) ToAMQPStructured(event *CloudEvent) (*AMQPMessage, error) {
	// Create combined structure
	combined := map[string]any{
		"specversion": event.SpecVersion,
		"id":          event.ID,
		"source":      event.Source,
		"type":        event.Type,
	}
	if event.Subject != "" {
		combined["subject"] = event.Subject
	}
	if event.Time != "" {
		combined["time"] = event.Time
	}
	if event.DataContentType != "" {
		combined["datacontenttype"] = event.DataContentType
	}
	if event.Data != nil {
		combined["data"] = event.Data
	}
	for k, v := range event.Extensions {
		combined[k] = v
	}

	jsonBytes, err := json.Marshal(combined)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}

	msg := NewAMQPMessage(jsonBytes)
	msg.Properties.ContentType = "application/cloudevents+json"

	return msg, nil
}

// FromAMQP converts AMQP message to CloudEvent
func (c *CloudEventsCodec) FromAMQP(msg *AMQPMessage) (*CloudEvent, error) {
	contentType := msg.Properties.ContentType

	// Structured mode
	if contentType == "application/cloudevents+json" {
		return c.fromStructured(msg)
	}

	// Binary mode (default)
	return c.fromBinary(msg)
}

func (c *CloudEventsCodec) fromStructured(msg *AMQPMessage) (*CloudEvent, error) {
	var combined map[string]any
	if err := json.Unmarshal(msg.Data[0], &combined); err != nil {
		return nil, fmt.Errorf("unmarshal structured event: %w", err)
	}

	event := &CloudEvent{
		Extensions: make(map[string]any),
	}

	// Extract standard attributes
	if v, ok := combined["specversion"].(string); ok {
		event.SpecVersion = v
	}
	if v, ok := combined["id"].(string); ok {
		event.ID = v
	}
	if v, ok := combined["source"].(string); ok {
		event.Source = v
	}
	if v, ok := combined["type"].(string); ok {
		event.Type = v
	}
	if v, ok := combined["subject"].(string); ok {
		event.Subject = v
	}
	if v, ok := combined["time"].(string); ok {
		event.Time = v
	}
	if v, ok := combined["datacontenttype"].(string); ok {
		event.DataContentType = v
	}
	event.Data = combined["data"]

	// Extract extensions (anything not standard)
	standardKeys := map[string]bool{
		"specversion": true, "id": true, "source": true, "type": true,
		"subject": true, "time": true, "datacontenttype": true, "data": true,
		"dataschema": true,
	}
	for k, v := range combined {
		if !standardKeys[k] {
			event.Extensions[k] = v
		}
	}

	return event, nil
}

func (c *CloudEventsCodec) fromBinary(msg *AMQPMessage) (*CloudEvent, error) {
	event := &CloudEvent{
		Extensions: make(map[string]any),
	}

	// Extract from application-properties
	for k, v := range msg.ApplicationProperties {
		if len(k) < 3 || k[:3] != "ce_" {
			continue
		}
		attr := k[3:] // Remove ce_ prefix

		strVal, _ := v.(string)
		switch attr {
		case "specversion":
			event.SpecVersion = strVal
		case "id":
			event.ID = strVal
		case "source":
			event.Source = strVal
		case "type":
			event.Type = strVal
		case "subject":
			event.Subject = strVal
		case "time":
			event.Time = strVal
		case "datacontenttype":
			event.DataContentType = strVal
		default:
			// Extension attribute
			event.Extensions[attr] = v
		}
	}

	// Parse data
	if len(msg.Data) > 0 && len(msg.Data[0]) > 0 {
		var data any
		if err := json.Unmarshal(msg.Data[0], &data); err != nil {
			// Keep as raw bytes if not JSON
			event.Data = msg.Data[0]
		} else {
			event.Data = data
		}
	}

	return event, nil
}

// ============================================================================
// GOENGINE AMQP ADAPTER
// ============================================================================

// AMQPConfig configures the AMQP adapter
type AMQPConfig struct {
	URL       string
	Queue     string
	Username  string
	Password  string
	Prefetch  uint32
	Reconnect bool
}

// AMQPSubscriber implements goengine Subscriber for AMQP 1.0
type AMQPSubscriber struct {
	config   AMQPConfig
	codec    *CloudEventsCodec
	conn     *Conn
	session  *Session
	receiver *Receiver
}

// NewAMQPSubscriber creates a new AMQP subscriber
func NewAMQPSubscriber(config AMQPConfig) *AMQPSubscriber {
	return &AMQPSubscriber{
		config: config,
		codec:  &CloudEventsCodec{},
	}
}

// Subscribe implements Subscriber[*CloudEvent]
func (s *AMQPSubscriber) Subscribe(ctx context.Context) <-chan *CloudEvent {
	out := make(chan *CloudEvent)

	go func() {
		defer close(out)

		// Connect
		opts := &ConnOptions{}
		if s.config.Username != "" {
			opts.SASLType = SASLTypePlain(s.config.Username, s.config.Password)
		}

		conn, err := Dial(ctx, s.config.URL, opts)
		if err != nil {
			fmt.Printf("[AMQP] Connection error: %v\n", err)
			return
		}
		defer conn.Close()
		s.conn = conn

		session, err := conn.NewSession(ctx, nil)
		if err != nil {
			fmt.Printf("[AMQP] Session error: %v\n", err)
			return
		}
		s.session = session

		receiver, err := session.NewReceiver(ctx, s.config.Queue, &ReceiverOptions{
			Credit: s.config.Prefetch,
		})
		if err != nil {
			fmt.Printf("[AMQP] Receiver error: %v\n", err)
			return
		}
		s.receiver = receiver

		fmt.Printf("[AMQP] Subscribed to %s\n", s.config.Queue)

		// Receive loop
		for {
			msg, err := receiver.Receive(ctx, nil)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				fmt.Printf("[AMQP] Receive error: %v\n", err)
				continue
			}

			// Convert to CloudEvent
			event, err := s.codec.FromAMQP(msg)
			if err != nil {
				fmt.Printf("[AMQP] Parse error: %v\n", err)
				receiver.RejectMessage(ctx, msg, &Error{
					Condition:   "amqp:decode-error",
					Description: err.Error(),
				})
				continue
			}

			// Send to channel
			select {
			case out <- event:
				receiver.AcceptMessage(ctx, msg)
			case <-ctx.Done():
				receiver.ReleaseMessage(ctx, msg)
				return
			}
		}
	}()

	return out
}

// AMQPPublisher implements goengine Publisher for AMQP 1.0
type AMQPPublisher struct {
	config  AMQPConfig
	codec   *CloudEventsCodec
	conn    *Conn
	session *Session
	sender  *Sender
	binary  bool // Use binary mode (default) vs structured
}

// NewAMQPPublisher creates a new AMQP publisher
func NewAMQPPublisher(config AMQPConfig) *AMQPPublisher {
	return &AMQPPublisher{
		config: config,
		codec:  &CloudEventsCodec{},
		binary: true, // Default to binary mode
	}
}

// Connect establishes connection to broker
func (p *AMQPPublisher) Connect(ctx context.Context) error {
	opts := &ConnOptions{}
	if p.config.Username != "" {
		opts.SASLType = SASLTypePlain(p.config.Username, p.config.Password)
	}

	conn, err := Dial(ctx, p.config.URL, opts)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	p.conn = conn

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	p.session = session

	sender, err := session.NewSender(ctx, p.config.Queue, nil)
	if err != nil {
		return fmt.Errorf("sender: %w", err)
	}
	p.sender = sender

	fmt.Printf("[AMQP] Connected to %s\n", p.config.Queue)
	return nil
}

// Publish sends a CloudEvent
func (p *AMQPPublisher) Publish(ctx context.Context, event *CloudEvent) error {
	var msg *AMQPMessage
	var err error

	if p.binary {
		msg, err = p.codec.ToAMQPBinary(event)
	} else {
		msg, err = p.codec.ToAMQPStructured(event)
	}
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	if err := p.sender.Send(ctx, msg, nil); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	return nil
}

// Close closes the connection
func (p *AMQPPublisher) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// ============================================================================
// EXAMPLE USAGE
// ============================================================================

// OrderCreated domain event
type OrderCreated struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Total      float64 `json:"total"`
}

// ExampleAMQPAdapter demonstrates AMQP 1.0 adapter usage
func ExampleAMQPAdapter() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Configuration for Azure Service Bus
	config := AMQPConfig{
		URL:      "amqps://my-namespace.servicebus.windows.net",
		Queue:    "orders",
		Username: "$ConnectionString",
		Password: "Endpoint=sb://...",
		Prefetch: 10,
	}

	fmt.Println("=== AMQP 1.0 Adapter Example ===\n")

	// Create publisher
	pub := NewAMQPPublisher(config)
	if err := pub.Connect(ctx); err != nil {
		fmt.Printf("Publisher connect error: %v\n", err)
		// Continue with demo
	}
	defer pub.Close()

	// Create CloudEvent
	event := &CloudEvent{
		SpecVersion:     "1.0",
		ID:              "evt-12345",
		Source:          "/orders/api",
		Type:            "com.example.order.created",
		Time:            time.Now().Format(time.RFC3339),
		DataContentType: "application/json",
		Data: OrderCreated{
			OrderID:    "ORD-001",
			CustomerID: "CUST-123",
			Total:      99.99,
		},
		Extensions: map[string]any{
			"xdestination": "kafka://analytics",
		},
	}

	// Publish
	fmt.Println("Publishing CloudEvent (binary mode)...")
	if err := pub.Publish(ctx, event); err != nil {
		fmt.Printf("Publish error: %v\n", err)
	}

	// Show AMQP message format
	codec := &CloudEventsCodec{}

	fmt.Println("\n--- Binary Mode AMQP Message ---")
	binaryMsg, _ := codec.ToAMQPBinary(event)
	fmt.Printf("ContentType: %s\n", binaryMsg.Properties.ContentType)
	fmt.Printf("Data: %s\n", string(binaryMsg.Data[0]))
	fmt.Println("ApplicationProperties:")
	for k, v := range binaryMsg.ApplicationProperties {
		fmt.Printf("  %s: %v\n", k, v)
	}

	fmt.Println("\n--- Structured Mode AMQP Message ---")
	structuredMsg, _ := codec.ToAMQPStructured(event)
	fmt.Printf("ContentType: %s\n", structuredMsg.Properties.ContentType)
	fmt.Printf("Data: %s\n", string(structuredMsg.Data[0]))

	// Demonstrate round-trip
	fmt.Println("\n--- Round-trip Conversion ---")
	recovered, _ := codec.FromAMQP(binaryMsg)
	fmt.Printf("Recovered Event:\n")
	fmt.Printf("  ID: %s\n", recovered.ID)
	fmt.Printf("  Type: %s\n", recovered.Type)
	fmt.Printf("  Source: %s\n", recovered.Source)
	fmt.Printf("  Data: %+v\n", recovered.Data)
	fmt.Printf("  Extensions: %v\n", recovered.Extensions)
}

// ExampleCompatibleBrokers shows which brokers work with go-amqp
func ExampleCompatibleBrokers() {
	fmt.Println("\n=== AMQP 1.0 Compatible Brokers ===")

	brokers := []struct {
		name string
		url  string
	}{
		{"Azure Service Bus", "amqps://<namespace>.servicebus.windows.net"},
		{"Azure Event Hubs", "amqps://<namespace>.servicebus.windows.net/<eventhub>"},
		{"Apache Qpid", "amqp://localhost:5672"},
		{"Apache ActiveMQ Artemis", "amqp://localhost:5672"},
		{"Red Hat AMQ", "amqps://broker.example.com:5671"},
	}

	for _, b := range brokers {
		fmt.Printf("✅ %s: %s\n", b.name, b.url)
	}

	fmt.Println("\n❌ NOT compatible (use amqp091-go instead):")
	fmt.Println("   RabbitMQ (uses AMQP 0.9.1 by default)")
}

func init() {
	ExampleAMQPAdapter()
	ExampleCompatibleBrokers()
}
