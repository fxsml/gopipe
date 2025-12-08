package broker

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

	// Marshaler for message serialization. Defaults to JSONMarshaler.
	Marshaler Marshaler

	// Client is the HTTP client to use for sending. Defaults to http.DefaultClient.
	Client *http.Client

	// Headers are additional headers to include in requests.
	Headers map[string]string

	// ContentType for the payload. Defaults to application/json.
	ContentType string
}

func (c *HTTPConfig) defaults() HTTPConfig {
	cfg := *c
	if cfg.Marshaler == nil {
		cfg.Marshaler = JSONMarshaler{}
	}
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	if cfg.ContentType == "" {
		cfg.ContentType = ContentTypeJSON
	}
	return cfg
}

// HTTPSender sends messages via HTTP POST to a webhook URL.
// Properties are sent as X-Gopipe-* headers, payload is sent as request body.
type HTTPSender[T any] struct {
	config    HTTPConfig
	url       string
	marshaler Marshaler
	client    *http.Client
}

// NewHTTPSender creates a sender that POSTs messages to the given URL.
// The topic is sent in X-Gopipe-Topic header.
// Properties are sent as X-Gopipe-Prop-* headers.
// Payload is sent as the request body with configured content type.
func NewHTTPSender[T any](url string, config HTTPConfig) *HTTPSender[T] {
	cfg := config.defaults()
	return &HTTPSender[T]{
		config:    cfg,
		url:       url,
		marshaler: cfg.Marshaler,
		client:    cfg.Client,
	}
}

// Send POSTs messages to the configured webhook URL.
// Each message is sent as a separate HTTP request.
func (s *HTTPSender[T]) Send(ctx context.Context, topic string, msgs ...*message.Message[T]) error {
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

func (s *HTTPSender[T]) sendOne(ctx context.Context, topic string, msg *message.Message[T]) error {
	// Marshal payload
	body, err := s.marshaler.Marshal(msg.Payload())
	if err != nil {
		return errors.Join(ErrMarshalFailed, err)
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
	msg.Properties().Range(func(key string, value any) bool {
		headerKey := propertyToHeader(key)
		req.Header.Set(headerKey, fmt.Sprintf("%v", value))
		return true
	})

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

// Close is a no-op for HTTPSender (implements Sender interface pattern).
func (s *HTTPSender[T]) Close() error {
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

// HTTPReceiver receives messages via HTTP POST requests.
// It implements an HTTP handler that accepts messages and emits them on channels.
type HTTPReceiver[T any] struct {
	config     HTTPConfig
	mu         sync.RWMutex
	topics     map[string]*httpTopic[T]
	closed     bool
	marshaler  Marshaler
	bufferSize int
}

type httpTopic[T any] struct {
	mu          sync.RWMutex
	subscribers map[*httpSubscriber[T]]struct{}
}

type httpSubscriber[T any] struct {
	ch     chan *message.Message[T]
	ctx    context.Context
	cancel context.CancelFunc
}

// NewHTTPReceiver creates a receiver that accepts messages via HTTP POST.
// Use ServeHTTP or Handler() to integrate with an HTTP server.
// Topic is read from X-Gopipe-Topic header.
// Properties are read from X-Gopipe-Prop-* headers.
// Payload is read from request body.
func NewHTTPReceiver[T any](config HTTPConfig, bufferSize int) *HTTPReceiver[T] {
	cfg := config.defaults()
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &HTTPReceiver[T]{
		config:     cfg,
		topics:     make(map[string]*httpTopic[T]),
		marshaler:  cfg.Marshaler,
		bufferSize: bufferSize,
	}
}

// Handler returns an http.Handler for receiving messages.
func (r *HTTPReceiver[T]) Handler() http.Handler {
	return http.HandlerFunc(r.ServeHTTP)
}

// ServeHTTP implements http.Handler for receiving messages via POST.
// Returns 201 Created on success, 400 Bad Request on invalid input,
// 405 Method Not Allowed for non-POST requests.
func (r *HTTPReceiver[T]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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

	// Unmarshal payload
	var payload T
	if err := r.marshaler.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Extract properties from headers
	opts := []message.Option[T]{}
	for key, values := range req.Header {
		if strings.HasPrefix(key, HeaderPrefix+"Prop-") && len(values) > 0 {
			propKey := headerToProperty(key)
			opts = append(opts, message.WithProperty[T](propKey, values[0]))
		}
	}

	msg := message.New(payload, opts...)

	// Broadcast to subscribers
	r.broadcast(topic, msg)

	w.WriteHeader(http.StatusCreated)
}

func (r *HTTPReceiver[T]) getOrCreateTopic(name string) *httpTopic[T] {
	r.mu.RLock()
	t, ok := r.topics[name]
	r.mu.RUnlock()
	if ok {
		return t
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok = r.topics[name]; ok {
		return t
	}
	t = &httpTopic[T]{
		subscribers: make(map[*httpSubscriber[T]]struct{}),
	}
	r.topics[name] = t
	return t
}

func (r *HTTPReceiver[T]) broadcast(topicName string, msg *message.Message[T]) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, topic := range r.topics {
		matcher := NewTopicMatcher(name)
		if matcher.Matches(topicName) {
			topic.mu.RLock()
			for sub := range topic.subscribers {
				select {
				case sub.ch <- msg:
				case <-sub.ctx.Done():
				default:
					// Drop if subscriber is slow
				}
			}
			topic.mu.RUnlock()
		}
	}
}

// Receive returns a channel that emits messages from the specified topic.
// The topic can include wildcards (+, #) for pattern matching.
func (r *HTTPReceiver[T]) Receive(ctx context.Context, topic string) <-chan *message.Message[T] {
	out := make(chan *message.Message[T], r.bufferSize)

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		close(out)
		return out
	}
	r.mu.RUnlock()

	subCtx, cancel := context.WithCancel(ctx)
	sub := &httpSubscriber[T]{
		ch:     make(chan *message.Message[T], r.bufferSize),
		ctx:    subCtx,
		cancel: cancel,
	}

	t := r.getOrCreateTopic(topic)
	t.mu.Lock()
	t.subscribers[sub] = struct{}{}
	t.mu.Unlock()

	go func() {
		defer close(out)
		defer func() {
			t.mu.Lock()
			delete(t.subscribers, sub)
			t.mu.Unlock()
			cancel()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.ch:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// Close shuts down the receiver and closes all subscriber channels.
func (r *HTTPReceiver[T]) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrHTTPServerClosed
	}
	r.closed = true

	for _, topic := range r.topics {
		topic.mu.Lock()
		for sub := range topic.subscribers {
			sub.cancel()
			close(sub.ch)
		}
		topic.mu.Unlock()
	}

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
