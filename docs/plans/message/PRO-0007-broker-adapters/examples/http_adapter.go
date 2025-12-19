// Package examples demonstrates CloudEvents HTTP adapter using SDK directly.
//
// The HTTP adapter is the one case where using the CloudEvents SDK protocol
// binding directly makes sense, because:
// - HTTP is stateless (no persistent connections to manage)
// - SDK callback pattern works well for webhooks
// - No complex acknowledgment semantics
package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ============================================================================
// MOCK CloudEvents SDK HTTP Types
// (In real code: import cloudevents "github.com/cloudevents/sdk-go/v2")
// ============================================================================

// Event is a mock of cloudevents.Event
type HTTPEvent struct {
	specVersion     string
	id              string
	source          string
	ceType          string
	subject         string
	time            time.Time
	dataContentType string
	data            []byte
	extensions      map[string]any
}

func NewHTTPEvent() *HTTPEvent {
	return &HTTPEvent{
		specVersion: "1.0",
		extensions:  make(map[string]any),
	}
}

func (e *HTTPEvent) SetID(id string)              { e.id = id }
func (e *HTTPEvent) SetSource(s string)           { e.source = s }
func (e *HTTPEvent) SetType(t string)             { e.ceType = t }
func (e *HTTPEvent) SetSubject(s string)          { e.subject = s }
func (e *HTTPEvent) SetTime(t time.Time)          { e.time = t }
func (e *HTTPEvent) SetDataContentType(ct string) { e.dataContentType = ct }

func (e *HTTPEvent) SetData(contentType string, payload any) error {
	e.dataContentType = contentType
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.data = data
	return nil
}

func (e *HTTPEvent) ID() string              { return e.id }
func (e *HTTPEvent) Source() string          { return e.source }
func (e *HTTPEvent) Type() string            { return e.ceType }
func (e *HTTPEvent) DataContentType() string { return e.dataContentType }

func (e *HTTPEvent) DataAs(target any) error {
	return json.Unmarshal(e.data, target)
}

// Client is a mock of cloudevents.Client
type CEClient struct {
	target   string
	port     int
	handlers []func(*HTTPEvent)
}

// ClientOption configures the client
type ClientOption func(*CEClient)

// WithTarget sets the target URL for sending
func WithTarget(target string) ClientOption {
	return func(c *CEClient) { c.target = target }
}

// WithPort sets the listening port for receiving
func WithPort(port int) ClientOption {
	return func(c *CEClient) { c.port = port }
}

// NewClient creates a CloudEvents HTTP client
func NewClient(opts ...ClientOption) *CEClient {
	c := &CEClient{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Send sends an event to the target
func (c *CEClient) Send(ctx context.Context, event *HTTPEvent) error {
	// In real SDK, this would make HTTP POST request
	fmt.Printf("[HTTP] Sending to %s:\n", c.target)
	fmt.Printf("  Type: %s\n", event.Type())
	fmt.Printf("  Source: %s\n", event.Source())
	return nil
}

// StartReceiver starts HTTP server and calls handler for each event
func (c *CEClient) StartReceiver(ctx context.Context, handler func(*HTTPEvent)) error {
	fmt.Printf("[HTTP] Starting receiver on port %d\n", c.port)

	// Simulate receiving events (in real SDK, this runs HTTP server)
	go func() {
		// Simulate some incoming events
		time.Sleep(100 * time.Millisecond)

		event := NewHTTPEvent()
		event.SetID("incoming-001")
		event.SetSource("/external/webhook")
		event.SetType("com.partner.notification")
		event.SetData("application/json", map[string]string{"message": "Hello from webhook"})

		handler(event)
	}()

	<-ctx.Done()
	return ctx.Err()
}

// ============================================================================
// GOENGINE HTTP ADAPTER (Using SDK Directly)
// ============================================================================

// HTTPSubscriberConfig configures HTTP receiver
type HTTPSubscriberConfig struct {
	Port    int
	Path    string // e.g., "/events"
	Handler func(event *HTTPEvent) error
}

// HTTPSubscriber receives CloudEvents via HTTP webhooks
type HTTPSubscriber struct {
	config HTTPSubscriberConfig
	client *CEClient
}

// NewHTTPSubscriber creates a new HTTP subscriber
func NewHTTPSubscriber(config HTTPSubscriberConfig) *HTTPSubscriber {
	return &HTTPSubscriber{
		config: config,
		client: NewClient(WithPort(config.Port)),
	}
}

// Subscribe implements Subscriber[*HTTPEvent]
func (s *HTTPSubscriber) Subscribe(ctx context.Context) <-chan *HTTPEvent {
	out := make(chan *HTTPEvent)

	go func() {
		defer close(out)

		err := s.client.StartReceiver(ctx, func(event *HTTPEvent) {
			select {
			case out <- event:
			case <-ctx.Done():
			}
		})
		if err != nil && err != context.Canceled {
			fmt.Printf("[HTTP] Receiver error: %v\n", err)
		}
	}()

	return out
}

// HTTPPublisherConfig configures HTTP sender
type HTTPPublisherConfig struct {
	TargetURL string
	Timeout   time.Duration
	Headers   map[string]string
}

// HTTPPublisher sends CloudEvents via HTTP POST
type HTTPPublisher struct {
	config HTTPPublisherConfig
	client *CEClient
}

// NewHTTPPublisher creates a new HTTP publisher
func NewHTTPPublisher(config HTTPPublisherConfig) *HTTPPublisher {
	return &HTTPPublisher{
		config: config,
		client: NewClient(WithTarget(config.TargetURL)),
	}
}

// Publish sends a CloudEvent via HTTP
func (p *HTTPPublisher) Publish(ctx context.Context, event *HTTPEvent) error {
	return p.client.Send(ctx, event)
}

// ============================================================================
// HTTP ENDPOINT HANDLER (For incoming webhooks)
// ============================================================================

// HTTPHandler handles incoming CloudEvents webhooks
type HTTPHandler struct {
	eventChan chan *HTTPEvent
}

// NewHTTPHandler creates a webhook handler
func NewHTTPHandler() *HTTPHandler {
	return &HTTPHandler{
		eventChan: make(chan *HTTPEvent, 100),
	}
}

// ServeHTTP implements http.Handler
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse CloudEvent from HTTP request
	// In real code: event, err := cloudevents.NewEventFromHTTPRequest(r)

	event := NewHTTPEvent()

	// Extract CE attributes from headers
	event.SetID(r.Header.Get("Ce-Id"))
	event.SetSource(r.Header.Get("Ce-Source"))
	event.SetType(r.Header.Get("Ce-Type"))

	// Read body
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	event.SetData("application/json", body)

	// Send to channel
	select {
	case h.eventChan <- event:
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Buffer full", http.StatusServiceUnavailable)
	}
}

// Events returns the channel of received events
func (h *HTTPHandler) Events() <-chan *HTTPEvent {
	return h.eventChan
}

// ============================================================================
// EXAMPLE USAGE
// ============================================================================

// WebhookPayload example payload
type WebhookPayload struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
}

// ExampleHTTPAdapter demonstrates HTTP adapter usage
func ExampleHTTPAdapter() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	fmt.Println("=== HTTP CloudEvents Adapter ===\n")

	// --- Publisher (sending webhooks) ---
	pub := NewHTTPPublisher(HTTPPublisherConfig{
		TargetURL: "https://partner.example.com/events",
		Timeout:   10 * time.Second,
	})

	event := NewHTTPEvent()
	event.SetID("evt-http-001")
	event.SetSource("/my-service/orders")
	event.SetType("com.example.order.shipped")
	event.SetTime(time.Now())
	event.SetData("application/json", WebhookPayload{
		Action:   "shipped",
		Resource: "order/ORD-123",
	})

	fmt.Println("--- Sending HTTP CloudEvent ---")
	if err := pub.Publish(ctx, event); err != nil {
		fmt.Printf("Publish error: %v\n", err)
	}

	// --- Subscriber (receiving webhooks) ---
	fmt.Println("\n--- Receiving HTTP CloudEvents ---")
	sub := NewHTTPSubscriber(HTTPSubscriberConfig{
		Port: 8080,
		Path: "/events",
	})

	events := sub.Subscribe(ctx)

	// Process incoming events
	for event := range events {
		fmt.Printf("Received event:\n")
		fmt.Printf("  ID: %s\n", event.ID())
		fmt.Printf("  Type: %s\n", event.Type())
		fmt.Printf("  Source: %s\n", event.Source())
	}
}

// ExampleHTTPIntegration shows how HTTP fits in goengine
func ExampleHTTPIntegration() {
	fmt.Println("\n=== HTTP Adapter Integration ===")
	fmt.Println(`
HTTP is recommended for:
  ✅ Receiving webhooks from external services
  ✅ Sending notifications to partners
  ✅ Serverless/FaaS event ingestion
  ✅ Simple request/response patterns

HTTP is NOT recommended for:
  ❌ High-throughput message streaming
  ❌ Guaranteed delivery (use message broker)
  ❌ Fan-out to multiple consumers (use pub/sub)

goengine architecture with HTTP:

  External Webhook                    Partner Webhook
        │                                   ▲
        ▼                                   │
  ┌─────────────┐                   ┌───────────────┐
  │ HTTP        │                   │ HTTP          │
  │ Subscriber  │                   │ Publisher     │
  └──────┬──────┘                   └───────▲───────┘
         │                                   │
         │      ┌─────────────────┐         │
         └─────►│     Engine      │─────────┘
                │  (internal loop)│
                └─────────────────┘
`)
}

func init() {
	ExampleHTTPAdapter()
	ExampleHTTPIntegration()
}
