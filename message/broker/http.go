package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/cloudevents"
)

var (
	// ErrHTTPRequestFailed is returned when an HTTP request fails.
	ErrHTTPRequestFailed = errors.New("HTTP request failed")
	// ErrHTTPReceiverClosed is returned when operations are attempted on a closed receiver.
	ErrHTTPReceiverClosed = errors.New("HTTP receiver is closed")
	// ErrInvalidCloudEvent is returned when a CloudEvent is invalid.
	ErrInvalidCloudEvent = errors.New("invalid CloudEvent")
)

const (
	// CloudEvents HTTP header prefix as per CloudEvents v1.0.2 spec.
	ceHeaderPrefix = "ce-"

	// CloudEvents required context attributes (as HTTP headers in binary mode).
	ceID          = ceHeaderPrefix + "id"
	ceSource      = ceHeaderPrefix + "source"
	ceSpecVersion = ceHeaderPrefix + "specversion"
	ceType        = ceHeaderPrefix + "type"

	// CloudEvents optional context attributes.
	ceSubject         = ceHeaderPrefix + "subject"
	ceTime            = ceHeaderPrefix + "time"
	ceDataContentType = ceHeaderPrefix + "datacontenttype"

	// gopipe extension attributes.
	ceTopic         = ceHeaderPrefix + "topic"
	ceCorrelationID = ceHeaderPrefix + "correlationid"
	ceDeadline      = ceHeaderPrefix + "deadline"

	// HTTP Content-Type header.
	headerContentType = "Content-Type"

	// CloudEvents content modes.
	contentTypeCloudEventsJSON      = "application/cloudevents+json"
	contentTypeCloudEventsBatchJSON = "application/cloudevents-batch+json"
	contentTypeJSON                 = "application/json"
	contentTypeOctetStream          = "application/octet-stream"

	// CloudEvents specification version.
	cloudEventsVersion = "1.0"
)

// CloudEventsMode defines the CloudEvents content mode for HTTP transport.
type CloudEventsMode string

const (
	// CloudEventsBinary mode: event data in body, attributes as ce- prefixed headers.
	CloudEventsBinary CloudEventsMode = "binary"
	// CloudEventsStructured mode: full CloudEvent JSON in body.
	CloudEventsStructured CloudEventsMode = "structured"
	// CloudEventsBatch mode: array of CloudEvents JSON in body.
	CloudEventsBatch CloudEventsMode = "batch"
)

// CloudEvents HTTP header names for binary content mode.
// These can be used to read CloudEvents attributes from HTTP requests.
const (
	HeaderID              = "Ce-Id"
	HeaderSource          = "Ce-Source"
	HeaderSpecVersion     = "Ce-Specversion"
	HeaderType            = "Ce-Type"
	HeaderSubject         = "Ce-Subject"
	HeaderTime            = "Ce-Time"
	HeaderDataContentType = "Ce-Datacontenttype"
	HeaderTopic           = "Ce-Topic"
	HeaderCorrelationID   = "Ce-Correlationid"
	HeaderDeadline        = "Ce-Deadline"
)

// HTTPConfig configures HTTP-based sender/receiver.
type HTTPConfig struct {
	// SendTimeout is the maximum duration for sending a message.
	SendTimeout time.Duration

	// Client is the HTTP client to use for sending. Defaults to http.DefaultClient.
	Client *http.Client

	// Headers are additional headers to include in requests.
	Headers map[string]string

	// ContentType for the payload in binary mode. Defaults to application/json.
	// Ignored in structured/batch modes.
	ContentType string

	// CloudEventsMode specifies the CloudEvents content mode. Default: binary.
	CloudEventsMode CloudEventsMode

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
		cfg.ContentType = contentTypeJSON
	}
	if cfg.CloudEventsMode == "" {
		cfg.CloudEventsMode = CloudEventsBinary
	}
	if cfg.AckTimeout == 0 {
		cfg.AckTimeout = 30 * time.Second
	}
	return cfg
}

// percentEncodeHeader encodes a string value for use in HTTP headers per CloudEvents spec.
// Characters outside printable ASCII (U+0021-U+007E) and space, quote, percent are percent-encoded.
func percentEncodeHeader(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if r == ' ' || r == '"' || r == '%' || r < 0x21 || r > 0x7E {
			// Encode as UTF-8 bytes, then percent-encode each byte
			for _, b := range []byte(string(r)) {
				buf.WriteString(fmt.Sprintf("%%%02X", b))
			}
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// percentDecodeHeader decodes a percent-encoded HTTP header value.
func percentDecodeHeader(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// HTTPSender sends messages via HTTP POST to a webhook URL.
type HTTPSender struct {
	config HTTPConfig
	url    string
	client *http.Client
}

// Compile-time interface assertion
var _ message.Sender = (*HTTPSender)(nil)

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

	switch s.config.CloudEventsMode {
	case CloudEventsBatch:
		return s.sendBatch(ctx, topic, msgs)
	case CloudEventsStructured:
		for _, msg := range msgs {
			if err := s.sendStructured(ctx, topic, msg); err != nil {
				return err
			}
		}
		return nil
	default: // CloudEventsBinary
		for _, msg := range msgs {
			if err := s.sendBinary(ctx, topic, msg); err != nil {
				return err
			}
		}
		return nil
	}
}

// sendBinary sends a message in CloudEvents binary content mode.
func (s *HTTPSender) sendBinary(ctx context.Context, topic string, msg *message.Message) error {
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return ErrSendTimeout
		}
		return ctx.Err()
	default:
	}

	body := msg.Data
	if len(body) == 0 {
		body = []byte("null")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	// Set Content-Type from config or datacontenttype attribute
	contentType := s.config.ContentType
	if dct, ok := msg.Attributes.DataContentType(); ok && dct != "" {
		contentType = dct
	}
	req.Header.Set(headerContentType, contentType)

	// Set required CloudEvents attributes
	s.setCloudEventsHeaders(req, topic, msg)

	// Set custom headers from config
	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}

	return s.doRequest(req)
}

// sendStructured sends a message in CloudEvents structured content mode.
func (s *HTTPSender) sendStructured(ctx context.Context, topic string, msg *message.Message) error {
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return ErrSendTimeout
		}
		return ctx.Err()
	default:
	}

	ce := s.messageToCloudEvent(topic, msg)
	body, err := json.Marshal(ce)
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	req.Header.Set(headerContentType, contentTypeCloudEventsJSON)

	// Set custom headers from config
	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}

	return s.doRequest(req)
}

// sendBatch sends multiple messages in CloudEvents batch content mode.
func (s *HTTPSender) sendBatch(ctx context.Context, topic string, msgs []*message.Message) error {
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return ErrSendTimeout
		}
		return ctx.Err()
	default:
	}

	events := make([]*cloudevents.Event, len(msgs))
	for i, msg := range msgs {
		events[i] = s.messageToCloudEvent(topic, msg)
	}

	body, err := json.Marshal(events)
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return errors.Join(ErrHTTPRequestFailed, err)
	}

	req.Header.Set(headerContentType, contentTypeCloudEventsBatchJSON)

	// Set custom headers from config
	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}

	return s.doRequest(req)
}

// setCloudEventsHeaders sets CloudEvents attributes as HTTP headers for binary mode.
func (s *HTTPSender) setCloudEventsHeaders(req *http.Request, topic string, msg *message.Message) {
	// Create CloudEvent and extract all attributes
	event := cloudevents.FromMessage(msg, topic, s.url)

	// Set required attributes
	req.Header.Set(ceID, percentEncodeHeader(event.ID))
	req.Header.Set(ceSource, percentEncodeHeader(event.Source))
	req.Header.Set(ceSpecVersion, event.SpecVersion)
	req.Header.Set(ceType, percentEncodeHeader(event.Type))

	// Set optional standard attributes
	if event.Subject != "" {
		req.Header.Set(ceSubject, percentEncodeHeader(event.Subject))
	}

	if event.Time != "" {
		req.Header.Set(ceTime, event.Time)
	}

	// Note: datacontenttype goes in Content-Type, not ce-datacontenttype header per spec

	// Set extension attributes
	for key, value := range event.Extensions {
		headerName := ceHeaderPrefix + strings.ToLower(key)
		req.Header.Set(headerName, percentEncodeHeader(fmt.Sprintf("%v", value)))
	}
}

// messageToCloudEvent converts a message to CloudEvent structure.
func (s *HTTPSender) messageToCloudEvent(topic string, msg *message.Message) *cloudevents.Event {
	return cloudevents.FromMessage(msg, topic, s.url)
}

// doRequest executes the HTTP request and checks the response.
func (s *HTTPSender) doRequest(req *http.Request) error {
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

// topicMessage holds a message with its topic.
type topicMessage struct {
	topic string
	msg   *message.Message
}

// HTTPReceiver receives messages via HTTP POST requests.
type HTTPReceiver struct {
	config            HTTPConfig
	mu                sync.Mutex
	messages          map[string][]topicMessage // keyed by topic
	readIndex         map[string]int            // track read position per topic
	closed            bool
	bufferSize        int
	maxBufferPerTopic int
}

// Compile-time interface assertion
var _ message.Receiver = (*HTTPReceiver)(nil)

// NewHTTPReceiver creates a receiver that accepts messages via HTTP POST.
// The bufferSize parameter controls the initial buffer capacity.
// Messages are limited to 10000 per topic to prevent unbounded memory growth.
func NewHTTPReceiver(config HTTPConfig, bufferSize int) *HTTPReceiver {
	cfg := config.defaults()
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &HTTPReceiver{
		config:            cfg,
		messages:          make(map[string][]topicMessage),
		readIndex:         make(map[string]int),
		bufferSize:        bufferSize,
		maxBufferPerTopic: 10000,
	}
}

// ackResult represents the result of waiting for message acknowledgment.
type ackResult struct {
	err error
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

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Detect CloudEvents mode by Content-Type
	contentType := req.Header.Get(headerContentType)

	var messages []topicMessage
	switch {
	case strings.HasPrefix(contentType, contentTypeCloudEventsBatchJSON):
		messages, err = r.parseBatchMode(req, body)
	case strings.HasPrefix(contentType, contentTypeCloudEventsJSON):
		msg, topic, err2 := r.parseStructuredMode(req, body)
		if err2 != nil {
			err = err2
		} else {
			messages = []topicMessage{{topic: topic, msg: msg}}
		}
	default:
		// Binary mode (default)
		msg, topic, err2 := r.parseBinaryMode(req, body)
		if err2 != nil {
			err = err2
		} else {
			messages = []topicMessage{{topic: topic, msg: msg}}
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create shared acking if WaitForAck is enabled
	var ackChan chan ackResult
	if r.config.WaitForAck && len(messages) > 0 {
		ackChan = make(chan ackResult, 1)

		// Create shared acking - ack fires when all messages acked, nack fires immediately
		acking := message.NewAcking(
			func() {
				select {
				case ackChan <- ackResult{}:
				default:
				}
			},
			func(err error) {
				select {
				case ackChan <- ackResult{err: err}:
				default:
				}
			},
			len(messages),
		)

		// Wrap each message with shared acking
		for i, tm := range messages {
			messages[i] = topicMessage{
				topic: tm.topic,
				msg:   message.NewWithSharedAcking(tm.msg.Data, tm.msg.Attributes, acking),
			}
		}
	}

	r.mu.Lock()
	for _, tm := range messages {
		msgs := r.messages[tm.topic]
		msgs = append(msgs, tm)
		// Enforce max buffer limit to prevent unbounded memory growth
		if len(msgs) > r.maxBufferPerTopic {
			// Adjust read index when evicting messages
			evictCount := len(msgs) - r.maxBufferPerTopic
			if r.readIndex[tm.topic] > 0 {
				r.readIndex[tm.topic] -= evictCount
				if r.readIndex[tm.topic] < 0 {
					r.readIndex[tm.topic] = 0
				}
			}
			msgs = msgs[evictCount:]
		}
		r.messages[tm.topic] = msgs
	}
	r.mu.Unlock()

	if r.config.WaitForAck && ackChan != nil {
		// Wait for acknowledgment (all messages acked or first nack)
		timeout := time.NewTimer(r.config.AckTimeout)
		defer timeout.Stop()

		select {
		case result := <-ackChan:
			if result.err != nil {
				http.Error(w, result.err.Error(), http.StatusInternalServerError)
				return
			}
		case <-timeout.C:
			http.Error(w, "acknowledgment timeout", http.StatusGatewayTimeout)
			return
		case <-req.Context().Done():
			http.Error(w, "request cancelled", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// parseBinaryMode parses a CloudEvent in binary content mode.
func (r *HTTPReceiver) parseBinaryMode(req *http.Request, body []byte) (*message.Message, string, error) {
	attrs := message.Attributes{}

	// Parse CloudEvents headers with ce- prefix
	for key, values := range req.Header {
		if len(values) == 0 {
			continue
		}

		lowerKey := strings.ToLower(key)
		if !strings.HasPrefix(lowerKey, ceHeaderPrefix) {
			continue
		}

		attrName := strings.TrimPrefix(lowerKey, ceHeaderPrefix)
		value := percentDecodeHeader(values[0])

		// Map CloudEvents attributes to message attributes
		switch attrName {
		case "id":
			attrs[message.AttrID] = value
		case "source":
			attrs[message.AttrSource] = value
		case "specversion":
			attrs[message.AttrSpecVersion] = value
		case "type":
			attrs[message.AttrType] = value
		case "subject":
			attrs[message.AttrSubject] = value
		case "time":
			if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
				attrs[message.AttrTime] = t
			}
		case "datacontenttype":
			attrs[message.AttrDataContentType] = value
		case "topic":
			attrs[message.AttrTopic] = value
		case "correlationid":
			attrs[message.AttrCorrelationID] = value
		case "deadline":
			if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
				attrs[message.AttrDeadline] = t
			}
		default:
			// Extension attribute
			attrs[attrName] = value
		}
	}

	// Validate required CloudEvents attributes
	if _, ok := attrs[message.AttrID]; !ok {
		return nil, "", fmt.Errorf("%w: missing required attribute 'id'", ErrInvalidCloudEvent)
	}
	if _, ok := attrs[message.AttrSource]; !ok {
		return nil, "", fmt.Errorf("%w: missing required attribute 'source'", ErrInvalidCloudEvent)
	}
	if _, ok := attrs[message.AttrType]; !ok {
		return nil, "", fmt.Errorf("%w: missing required attribute 'type'", ErrInvalidCloudEvent)
	}

	// Get topic from ce-topic header or URL path
	topic, _ := attrs.String(message.AttrTopic)
	if topic == "" {
		topic = TopicFromPath(req.URL.Path)
	}

	msg := message.New(body, attrs)
	return msg, topic, nil
}

// parseStructuredMode parses a CloudEvent in structured content mode (JSON).
func (r *HTTPReceiver) parseStructuredMode(req *http.Request, body []byte) (*message.Message, string, error) {
	var ce map[string]any
	if err := json.Unmarshal(body, &ce); err != nil {
		return nil, "", fmt.Errorf("%w: invalid JSON: %v", ErrInvalidCloudEvent, err)
	}

	return r.cloudEventToMessage(ce)
}

// parseBatchMode parses multiple CloudEvents in batch content mode.
func (r *HTTPReceiver) parseBatchMode(req *http.Request, body []byte) ([]topicMessage, error) {
	var batch []map[string]any
	if err := json.Unmarshal(body, &batch); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON batch: %v", ErrInvalidCloudEvent, err)
	}

	messages := make([]topicMessage, 0, len(batch))
	for i, ce := range batch {
		msg, topic, err := r.cloudEventToMessage(ce)
		if err != nil {
			return nil, fmt.Errorf("%w: event %d: %v", ErrInvalidCloudEvent, i, err)
		}
		messages = append(messages, topicMessage{topic: topic, msg: msg})
	}

	return messages, nil
}

// cloudEventToMessage converts a CloudEvent JSON object to a message.
func (r *HTTPReceiver) cloudEventToMessage(ce map[string]any) (*message.Message, string, error) {
	// Marshal the map to JSON, then unmarshal into CloudEvent struct
	data, err := json.Marshal(ce)
	if err != nil {
		return nil, "", fmt.Errorf("%w: failed to marshal CE map: %v", ErrInvalidCloudEvent, err)
	}

	var event cloudevents.Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidCloudEvent, err)
	}

	msg, topic, err := cloudevents.ToMessage(&event)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidCloudEvent, err)
	}

	return msg, topic, nil
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
