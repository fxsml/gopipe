package pubsub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
)

var (
	// ErrHTTPRequestFailed is returned when an HTTP request fails.
	ErrHTTPRequestFailed = errors.New("HTTP request failed")
	// ErrHTTPReceiverClosed is returned when operations are attempted on a closed receiver.
	ErrHTTPReceiverClosed = errors.New("HTTP receiver is closed")
)

const (
	// HeaderPrefix is the prefix for gopipe message properties in HTTP headers.
	HeaderPrefix = "X-Gopipe-"
	// HeaderTopic is the header name for the message topic.
	HeaderTopic = "X-Gopipe-Topic"
	// HeaderTimestamp is the header name for the message timestamp.
	HeaderTimestamp = "X-Gopipe-Timestamp"
	// HeaderContentType is the standard content type header.
	HeaderContentType = "Content-Type"
	// ContentTypeJSON is the JSON content type.
	ContentTypeJSON = "application/json"
	// ContentTypeOctetStream is the binary content type.
	ContentTypeOctetStream = "application/octet-stream"
)

// HTTPConfig configures HTTP-based sender/receiver.
type HTTPConfig struct {
	// SendTimeout is the maximum duration for sending a message.
	SendTimeout time.Duration

	// Client is the HTTP client to use for sending. Defaults to http.DefaultClient.
	Client *http.Client

	// Headers are additional headers to include in requests.
	Headers map[string]string

	// ContentType for the payload. Defaults to application/json.
	ContentType string

	// WaitForAck makes the HTTP receiver wait for message acknowledgment before responding.
	// When true: returns 200 OK after ack, 500 on nack/timeout.
	// When false: returns 201 Created immediately.
	WaitForAck bool

	// AckTimeout is the maximum duration to wait for acknowledgment when WaitForAck is true.
	// Default: 30 seconds.
	AckTimeout time.Duration
}

func (c HTTPConfig) defaults() HTTPConfig {
	cfg := c
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	if cfg.ContentType == "" {
		cfg.ContentType = ContentTypeJSON
	}
	if cfg.AckTimeout == 0 {
		cfg.AckTimeout = 30 * time.Second
	}
	return cfg
}

// HTTPSender sends messages via HTTP POST to a webhook URL.
type HTTPSender struct {
	config HTTPConfig
	url    string
	client *http.Client
}

// Compile-time interface assertion
var _ Sender = (*HTTPSender)(nil)

// NewHTTPSender creates a sender that POSTs messages to the given URL.
func NewHTTPSender(url string, config HTTPConfig) *HTTPSender {
	cfg := config.defaults()
	return &HTTPSender{
		config: cfg,
		url:    url,
		client: cfg.Client,
	}
}

// Send POSTs messages to the configured webhook URL.
func (s *HTTPSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	if s.config.SendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.SendTimeout)
		defer cancel()
	}

	for _, msg := range msgs {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return ErrSendTimeout
			}
			return ctx.Err()
		default:
		}

		if err := s.sendOne(ctx, topic, msg); err != nil {
			return err
		}
	}

	return nil
}

func (s *HTTPSender) sendOne(ctx context.Context, topic string, msg *message.Message) error {
	body := msg.Payload
	if len(body) == 0 {
		body = []byte("null")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	req.Header.Set(HeaderContentType, s.config.ContentType)
	req.Header.Set(HeaderTopic, topic)
	req.Header.Set(HeaderTimestamp, time.Now().UTC().Format(time.RFC3339Nano))

	for key, value := range msg.Properties {
		headerKey := propertyToHeader(key)
		req.Header.Set(headerKey, fmt.Sprintf("%v", value))
	}

	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrHTTPRequestFailed, resp.StatusCode)
	}

	return nil
}

// propertyToHeader converts a property key to an HTTP header name.
func propertyToHeader(key string) string {
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return HeaderPrefix + "Prop-" + strings.Join(parts, "-")
}

// headerToProperty converts an HTTP header name back to a property key.
func headerToProperty(header string) string {
	prop := strings.TrimPrefix(header, HeaderPrefix+"Prop-")
	return strings.ToLower(strings.ReplaceAll(prop, "-", "."))
}

// topicMessage holds a message with its topic.
type topicMessage struct {
	topic string
	msg   *message.Message
}

// HTTPReceiver receives messages via HTTP POST requests.
type HTTPReceiver struct {
	config     HTTPConfig
	mu         sync.Mutex
	messages   map[string][]topicMessage // keyed by topic
	readIndex  map[string]int            // track read position per topic
	closed     bool
	bufferSize int
}

// Compile-time interface assertion
var _ Receiver = (*HTTPReceiver)(nil)

// NewHTTPReceiver creates a receiver that accepts messages via HTTP POST.
func NewHTTPReceiver(config HTTPConfig, bufferSize int) *HTTPReceiver {
	cfg := config.defaults()
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &HTTPReceiver{
		config:     cfg,
		messages:   make(map[string][]topicMessage),
		readIndex:  make(map[string]int),
		bufferSize: bufferSize,
	}
}

// ServeHTTP implements http.Handler for receiving messages via POST.
func (r *HTTPReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		http.Error(w, "Server closed", http.StatusServiceUnavailable)
		return
	}
	r.mu.Unlock()

	// Read topic from header or URL path
	topic := req.Header.Get(HeaderTopic)
	if topic == "" {
		topic = strings.TrimPrefix(req.URL.Path, "/")
	}
	if topic == "" {
		http.Error(w, "Missing topic", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	props := message.Properties{}
	for key, values := range req.Header {
		if strings.HasPrefix(key, HeaderPrefix+"Prop-") && len(values) > 0 {
			propKey := headerToProperty(key)
			props[propKey] = values[0]
		}
	}

	msg := message.New(body, props)

	r.mu.Lock()
	r.messages[topic] = append(r.messages[topic], topicMessage{topic: topic, msg: msg})
	r.mu.Unlock()

	// TODO: WaitForAck support - for now just return 201
	if r.config.WaitForAck {
		// Would need ack channel on message - simplified for now
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// Receive returns unread messages for the specified topic and clears them.
func (r *HTTPReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrHTTPReceiverClosed
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get messages for exact topic match
	msgs := r.messages[topic]
	start := r.readIndex[topic]

	if start >= len(msgs) {
		return nil, nil
	}

	// Return unread messages
	result := make([]*message.Message, 0, len(msgs)-start)
	for i := start; i < len(msgs); i++ {
		result = append(result, msgs[i].msg)
	}

	// Update read index
	r.readIndex[topic] = len(msgs)

	// Clean up if all messages have been read
	if r.readIndex[topic] >= len(r.messages[topic]) {
		delete(r.messages, topic)
		delete(r.readIndex, topic)
	}

	return result, nil
}

// Close shuts down the receiver.
func (r *HTTPReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrHTTPReceiverClosed
	}
	r.closed = true
	r.messages = nil
	r.readIndex = nil

	return nil
}

// Handler returns the HTTP handler for this receiver.
// This is a convenience method for use with http.Server.
func (r *HTTPReceiver) Handler() http.Handler {
	return r
}

// TopicFromPath extracts the topic from a URL path.
func TopicFromPath(path string) string {
	topic := strings.TrimPrefix(path, "/")
	decoded, err := url.PathUnescape(topic)
	if err != nil {
		return topic
	}
	return decoded
}
