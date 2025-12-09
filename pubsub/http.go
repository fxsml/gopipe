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
	// ErrHTTPServerClosed is returned when operations are attempted on a closed server.
	ErrHTTPServerClosed = errors.New("HTTP server is closed")
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
}

func (c HTTPConfig) defaults() HTTPConfig {
	cfg := c
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	if cfg.ContentType == "" {
		cfg.ContentType = ContentTypeJSON
	}
	return cfg
}

// httpSender sends messages via HTTP POST to a webhook URL.
// Properties are sent as X-Gopipe-* headers, payload is sent as request body.
type httpSender struct {
	config HTTPConfig
	url    string
	client *http.Client
}

// NewHTTPSender creates a sender that POSTs messages to the given URL.
// The topic is sent in X-Gopipe-Topic header.
// Properties are sent as X-Gopipe-Prop-* headers.
// Payload is sent as the request body with configured content type.
func NewHTTPSender(url string, config HTTPConfig) message.Sender {
	cfg := config.defaults()
	return &httpSender{
		config: cfg,
		url:    url,
		client: cfg.Client,
	}
}

// Send POSTs messages to the configured webhook URL.
// Each message is sent as a separate HTTP request.
func (s *httpSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	// Apply timeout if configured
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

func (s *httpSender) sendOne(ctx context.Context, topic string, msg *message.Message) error {
	// Payload is already []byte
	body := msg.Payload
	if len(body) == 0 {
		body = []byte("null")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	// Set content type
	req.Header.Set(HeaderContentType, s.config.ContentType)

	// Set topic header
	req.Header.Set(HeaderTopic, topic)

	// Set timestamp header
	req.Header.Set(HeaderTimestamp, time.Now().UTC().Format(time.RFC3339Nano))

	// Set properties as headers
	for key, value := range msg.Properties {
		headerKey := propertyToHeader(key)
		req.Header.Set(headerKey, fmt.Sprintf("%v", value))
	}

	// Set custom headers
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
// e.g., "gopipe.message.id" -> "X-Gopipe-Prop-Gopipe-Message-Id"
func propertyToHeader(key string) string {
	// Replace dots and underscores with dashes, capitalize words
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
// e.g., "X-Gopipe-Prop-Gopipe-Message-Id" -> "gopipe.message.id"
func headerToProperty(header string) string {
	// Remove prefix
	prop := strings.TrimPrefix(header, HeaderPrefix+"Prop-")
	// Convert dashes to dots and lowercase
	return strings.ToLower(strings.ReplaceAll(prop, "-", "."))
}

// httpReceiver receives messages via HTTP POST requests.
// It implements an HTTP handler that accepts messages and buffers them.
type httpReceiver struct {
	config     HTTPConfig
	mu         sync.RWMutex
	messages   []topicMessage
	closed     bool
	bufferSize int
}

type topicMessage struct {
	topic string
	msg   *message.Message
}

// NewHTTPReceiver creates a receiver that accepts messages via HTTP POST.
// Use ServeHTTP or Handler() to integrate with an HTTP server.
// Topic is read from X-Gopipe-Topic header.
// Properties are read from X-Gopipe-Prop-* headers.
// Payload is read from request body.
func NewHTTPReceiver(config HTTPConfig, bufferSize int) message.Receiver {
	cfg := config.defaults()
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &httpReceiver{
		config:     cfg,
		messages:   make([]topicMessage, 0),
		bufferSize: bufferSize,
	}
}

// Handler returns an http.Handler for receiving messages.
func (r *httpReceiver) Handler() http.Handler {
	return http.HandlerFunc(r.ServeHTTP)
}

// ServeHTTP implements http.Handler for receiving messages via POST.
// Returns 201 Created on success, 400 Bad Request on invalid input,
// 405 Method Not Allowed for non-POST requests.
func (r *httpReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		http.Error(w, "Server closed", http.StatusServiceUnavailable)
		return
	}
	r.mu.RUnlock()

	// Read topic from header
	topic := req.Header.Get(HeaderTopic)
	if topic == "" {
		// Try to get topic from URL path
		topic = strings.TrimPrefix(req.URL.Path, "/")
	}
	if topic == "" {
		http.Error(w, "Missing topic", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Extract properties from headers
	props := message.Properties{}
	for key, values := range req.Header {
		if strings.HasPrefix(key, HeaderPrefix+"Prop-") && len(values) > 0 {
			propKey := headerToProperty(key)
			props[propKey] = values[0]
		}
	}

	msg := message.New(body, props)

	// Broadcast to subscribers
	r.broadcast(topic, msg)

	w.WriteHeader(http.StatusCreated)
}

func (r *httpReceiver) broadcast(topicName string, msg *message.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, topicMessage{topic: topicName, msg: msg})
}

// Receive returns all messages received for the specified topic.
// The topic can include wildcards (+, #) for pattern matching.
func (r *httpReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, ErrHTTPServerClosed
	}
	r.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Filter messages by topic pattern
	r.mu.RLock()
	defer r.mu.RUnlock()

	matcher := NewTopicMatcher(topic)
	result := make([]*message.Message, 0)
	for _, tm := range r.messages {
		if matcher.Matches(tm.topic) {
			result = append(result, tm.msg)
		}
	}
	return result, nil
}

// Close shuts down the receiver.
func (r *httpReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrHTTPServerClosed
	}
	r.closed = true

	return nil
}

// TopicFromPath extracts the topic from a URL path, URL-decoding it.
func TopicFromPath(path string) string {
	topic := strings.TrimPrefix(path, "/")
	decoded, err := url.PathUnescape(topic)
	if err != nil {
		return topic
	}
	return decoded
}
