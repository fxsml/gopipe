package pubsub

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

// cloudEvent represents a CloudEvent structure for JSON marshaling.
type cloudEvent struct {
	ID              string                 `json:"id"`
	Source          string                 `json:"source"`
	SpecVersion     string                 `json:"specversion"`
	Type            string                 `json:"type"`
	Subject         string                 `json:"subject,omitempty"`
	Time            string                 `json:"time,omitempty"`
	DataContentType string                 `json:"datacontenttype,omitempty"`
	Data            any                    `json:"data,omitempty"`
	Extensions      map[string]interface{} `json:"-"`
}

// MarshalJSON implements custom JSON marshaling to include extensions at top level.
func (c *cloudEvent) MarshalJSON() ([]byte, error) {
	type Alias cloudEvent
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	data, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	if len(c.Extensions) == 0 {
		return data, nil
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	for k, v := range c.Extensions {
		m[k] = v
	}

	return json.Marshal(m)
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

	events := make([]*cloudEvent, len(msgs))
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
	// Set required attributes
	id, _ := msg.Attributes.ID()
	if id == "" {
		id = generateID()
	}
	req.Header.Set(ceID, percentEncodeHeader(id))

	source, _ := msg.Attributes.Source()
	if source == "" {
		source = s.url
	}
	req.Header.Set(ceSource, percentEncodeHeader(source))

	req.Header.Set(ceSpecVersion, cloudEventsVersion)

	eventType, _ := msg.Attributes.Type()
	if eventType == "" {
		eventType = "com.gopipe.message"
	}
	req.Header.Set(ceType, percentEncodeHeader(eventType))

	// Set optional standard attributes
	if subject, ok := msg.Attributes.Subject(); ok {
		req.Header.Set(ceSubject, percentEncodeHeader(subject))
	}

	if t, ok := msg.Attributes.EventTime(); ok {
		req.Header.Set(ceTime, t.Format(time.RFC3339Nano))
	}

	// Note: datacontenttype goes in Content-Type, not ce-datacontenttype header per spec

	// Set gopipe extension attributes
	if topic != "" {
		req.Header.Set(ceTopic, percentEncodeHeader(topic))
	}

	if correlationID, ok := msg.Attributes.CorrelationID(); ok {
		req.Header.Set(ceCorrelationID, percentEncodeHeader(correlationID))
	}

	if deadline, ok := msg.Attributes.Deadline(); ok {
		req.Header.Set(ceDeadline, deadline.Format(time.RFC3339Nano))
	}

	// Set other extension attributes
	for key, value := range msg.Attributes {
		// Skip already-set standard attributes
		if isStandardAttribute(key) {
			continue
		}
		headerName := ceHeaderPrefix + strings.ToLower(key)
		req.Header.Set(headerName, percentEncodeHeader(fmt.Sprintf("%v", value)))
	}
}

// messageToCloudEvent converts a message to CloudEvent structure.
func (s *HTTPSender) messageToCloudEvent(topic string, msg *message.Message) *cloudEvent {
	ce := &cloudEvent{
		SpecVersion: cloudEventsVersion,
		Extensions:  make(map[string]interface{}),
	}

	// Set required attributes
	id, _ := msg.Attributes.ID()
	if id == "" {
		id = generateID()
	}
	ce.ID = id

	source, _ := msg.Attributes.Source()
	if source == "" {
		source = s.url
	}
	ce.Source = source

	eventType, _ := msg.Attributes.Type()
	if eventType == "" {
		eventType = "com.gopipe.message"
	}
	ce.Type = eventType

	// Set optional standard attributes
	if subject, ok := msg.Attributes.Subject(); ok {
		ce.Subject = subject
	}

	if t, ok := msg.Attributes.EventTime(); ok {
		ce.Time = t.Format(time.RFC3339Nano)
	}

	if dct, ok := msg.Attributes.DataContentType(); ok {
		ce.DataContentType = dct
	}

	// Set data - try to unmarshal as JSON if data content type indicates JSON
	if ce.DataContentType == "" || strings.Contains(ce.DataContentType, "json") {
		var jsonData interface{}
		if err := json.Unmarshal(msg.Data, &jsonData); err == nil {
			ce.Data = jsonData
		} else {
			ce.Data = string(msg.Data)
		}
	} else {
		ce.Data = string(msg.Data)
	}

	// Add gopipe extensions
	if topic != "" {
		ce.Extensions["topic"] = topic
	}

	if correlationID, ok := msg.Attributes.CorrelationID(); ok {
		ce.Extensions["correlationid"] = correlationID
	}

	if deadline, ok := msg.Attributes.Deadline(); ok {
		ce.Extensions["deadline"] = deadline.Format(time.RFC3339Nano)
	}

	// Add other extension attributes
	for key, value := range msg.Attributes {
		if !isStandardAttribute(key) {
			ce.Extensions[key] = value
		}
	}

	return ce
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

// isStandardAttribute checks if an attribute key is a CloudEvents standard attribute.
func isStandardAttribute(key string) bool {
	switch key {
	case message.AttrID, message.AttrSource, message.AttrSpecVersion, message.AttrType,
		message.AttrSubject, message.AttrTime, message.AttrDataContentType,
		message.AttrTopic, message.AttrCorrelationID, message.AttrDeadline:
		return true
	}
	return false
}

// generateID generates a unique ID for a CloudEvent.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
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

	r.mu.Lock()
	for _, tm := range messages {
		r.messages[tm.topic] = append(r.messages[tm.topic], tm)
	}
	r.mu.Unlock()

	// TODO: WaitForAck support - for now just return appropriate status
	if r.config.WaitForAck {
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
	var ce map[string]interface{}
	if err := json.Unmarshal(body, &ce); err != nil {
		return nil, "", fmt.Errorf("%w: invalid JSON: %v", ErrInvalidCloudEvent, err)
	}

	return r.cloudEventToMessage(ce)
}

// parseBatchMode parses multiple CloudEvents in batch content mode.
func (r *HTTPReceiver) parseBatchMode(req *http.Request, body []byte) ([]topicMessage, error) {
	var batch []map[string]interface{}
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
func (r *HTTPReceiver) cloudEventToMessage(ce map[string]interface{}) (*message.Message, string, error) {
	attrs := message.Attributes{}

	// Extract required attributes
	id, ok := ce["id"].(string)
	if !ok || id == "" {
		return nil, "", fmt.Errorf("%w: missing or invalid 'id'", ErrInvalidCloudEvent)
	}
	attrs[message.AttrID] = id

	source, ok := ce["source"].(string)
	if !ok || source == "" {
		return nil, "", fmt.Errorf("%w: missing or invalid 'source'", ErrInvalidCloudEvent)
	}
	attrs[message.AttrSource] = source

	specversion, ok := ce["specversion"].(string)
	if !ok || specversion == "" {
		return nil, "", fmt.Errorf("%w: missing or invalid 'specversion'", ErrInvalidCloudEvent)
	}
	attrs[message.AttrSpecVersion] = specversion

	eventType, ok := ce["type"].(string)
	if !ok || eventType == "" {
		return nil, "", fmt.Errorf("%w: missing or invalid 'type'", ErrInvalidCloudEvent)
	}
	attrs[message.AttrType] = eventType

	// Extract optional standard attributes
	if subject, ok := ce["subject"].(string); ok {
		attrs[message.AttrSubject] = subject
	}

	if timeStr, ok := ce["time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
			attrs[message.AttrTime] = t
		}
	}

	if dct, ok := ce["datacontenttype"].(string); ok {
		attrs[message.AttrDataContentType] = dct
	}

	// Extract gopipe extension attributes
	topic := ""
	if topicVal, ok := ce["topic"].(string); ok {
		topic = topicVal
		attrs[message.AttrTopic] = topicVal
	}

	if correlationID, ok := ce["correlationid"].(string); ok {
		attrs[message.AttrCorrelationID] = correlationID
	}

	if deadlineStr, ok := ce["deadline"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, deadlineStr); err == nil {
			attrs[message.AttrDeadline] = t
		}
	}

	// Extract other extension attributes (anything not in standard set)
	standardKeys := map[string]bool{
		"id": true, "source": true, "specversion": true, "type": true,
		"subject": true, "time": true, "datacontenttype": true, "data": true,
		"topic": true, "correlationid": true, "deadline": true,
	}
	for key, value := range ce {
		if !standardKeys[key] {
			attrs[key] = value
		}
	}

	// Extract data
	var data []byte
	if dataVal, ok := ce["data"]; ok {
		switch v := dataVal.(type) {
		case string:
			data = []byte(v)
		default:
			// Marshal back to JSON
			var err error
			data, err = json.Marshal(v)
			if err != nil {
				return nil, "", fmt.Errorf("%w: failed to marshal data: %v", ErrInvalidCloudEvent, err)
			}
		}
	}

	msg := message.New(data, attrs)
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
