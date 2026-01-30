package http

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
)

// SubscriberConfig configures an HTTP CloudEvents Subscriber.
type SubscriberConfig struct {
	// Addr is the HTTP server listen address (e.g., ":8080").
	// Required if using Start().
	Addr string

	// Path is the base path for receiving events (default: "/").
	// Topic paths are appended: "/events" + "/orders" → "/events/orders".
	Path string

	// BufferSize is the channel buffer size per topic (default: 100).
	BufferSize int

	// ReadTimeout is the HTTP request read timeout (default: 30s).
	ReadTimeout time.Duration

	// WriteTimeout is the HTTP response write timeout (default: 30s).
	WriteTimeout time.Duration

	// AckTimeout is the maximum time to wait for ack/nack (default: 30s).
	AckTimeout time.Duration

	// ErrorHandler is called on parse/validation errors.
	ErrorHandler func(err error)

	// Logger for structured logging.
	Logger message.Logger
}

func (c SubscriberConfig) parse() SubscriberConfig {
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if c.AckTimeout <= 0 {
		c.AckTimeout = 30 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// topicHandler holds the channel and context for a subscribed topic.
type topicHandler struct {
	ch     chan *message.RawMessage
	ctx    context.Context
	cancel context.CancelFunc
}

// Subscriber receives CloudEvents over HTTP and provides topic-based subscription.
// Each HTTP request runs in its own goroutine (unlimited concurrency).
type Subscriber struct {
	cfg    SubscriberConfig
	logger message.Logger

	mu       sync.RWMutex
	topics   map[string]*topicHandler
	server   *http.Server
	listener net.Listener
}

// NewSubscriber creates an HTTP CloudEvents subscriber.
func NewSubscriber(cfg SubscriberConfig) *Subscriber {
	cfg = cfg.parse()
	return &Subscriber{
		cfg:    cfg,
		logger: cfg.Logger,
		topics: make(map[string]*topicHandler),
	}
}

// Subscribe returns a channel for receiving CloudEvents on the given topic.
// Topic maps to HTTP path: cfg.Path + "/" + topic (e.g., "/events/orders").
// The channel is closed when the context is cancelled.
// Returns error if already subscribed to this topic.
func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.topics[topic]; exists {
		return nil, errors.New("already subscribed to topic: " + topic)
	}

	ch := make(chan *message.RawMessage, s.cfg.BufferSize)
	topicCtx, cancel := context.WithCancel(ctx)

	s.topics[topic] = &topicHandler{
		ch:     ch,
		ctx:    topicCtx,
		cancel: cancel,
	}

	// Handle context cancellation
	go func() {
		<-topicCtx.Done()
		s.mu.Lock()
		delete(s.topics, topic)
		close(ch)
		s.mu.Unlock()
	}()

	return ch, nil
}

// ServeHTTP implements http.Handler for embedding in existing servers.
func (s *Subscriber) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract topic from path
	topic := strings.TrimPrefix(r.URL.Path, s.cfg.Path)
	topic = strings.TrimPrefix(topic, "/")

	s.mu.RLock()
	handler, exists := s.topics[topic]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "topic not found: "+topic, http.StatusNotFound)
		return
	}

	// Check content type for batch vs single
	contentType := r.Header.Get("Content-Type")
	isBatch := strings.Contains(contentType, "application/cloudevents-batch+json")

	var msgs []*message.RawMessage
	var err error

	if isBatch {
		msgs, err = ParseBatch(r.Body)
	} else {
		var msg *message.RawMessage
		msg, err = message.ParseRaw(r.Body)
		if err == nil {
			msgs = []*message.RawMessage{msg}
		}
	}

	if err != nil {
		s.logger.Error("Failed to parse CloudEvent", "component", "http-subscriber", "error", err, "topic", topic)
		if s.cfg.ErrorHandler != nil {
			s.cfg.ErrorHandler(err)
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(msgs) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Create acking that tracks all messages
	var ackErr error
	var ackOnce sync.Once
	done := make(chan struct{})
	pending := len(msgs)
	var pendingMu sync.Mutex

	onAck := func() {
		pendingMu.Lock()
		pending--
		allDone := pending == 0
		pendingMu.Unlock()
		if allDone {
			close(done)
		}
	}

	onNack := func(err error) {
		ackOnce.Do(func() {
			ackErr = err
		})
		pendingMu.Lock()
		pending--
		allDone := pending == 0
		pendingMu.Unlock()
		if allDone {
			close(done)
		}
	}

	// Send messages to topic channel
	for _, msg := range msgs {
		acking := message.NewAcking(onAck, onNack)
		msgWithAck := message.NewRaw(msg.Data, msg.Attributes, acking)

		select {
		case handler.ch <- msgWithAck:
		case <-handler.ctx.Done():
			http.Error(w, "subscriber closed", http.StatusServiceUnavailable)
			return
		case <-r.Context().Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
	}

	// Wait for acks with timeout
	ackTimeout := s.cfg.AckTimeout
	select {
	case <-done:
		if ackErr != nil {
			http.Error(w, ackErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	case <-time.After(ackTimeout):
		http.Error(w, "ack timeout", http.StatusGatewayTimeout)
	case <-r.Context().Done():
		http.Error(w, "request cancelled", http.StatusRequestTimeout)
	}
}

// Handler returns an http.Handler for a specific topic.
// Useful when you need fine-grained routing control.
func (s *Subscriber) Handler(topic string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Override path to match topic
		r.URL.Path = s.cfg.Path + "/" + topic
		s.ServeHTTP(w, r)
	})
}

// Start starts the HTTP server. Blocks until context cancellation or error.
// Use ServeHTTP() instead if you have an existing server.
func (s *Subscriber) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		return errors.New("server already started")
	}

	mux := http.NewServeMux()
	mux.Handle(s.cfg.Path+"/", s)

	s.server = &http.Server{
		Addr:         s.cfg.Addr,
		Handler:      mux,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}

	// Create listener to get actual address
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.listener = ln
	s.mu.Unlock()

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	err = s.server.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Addr returns the listener address. Only valid after Start() is called.
func (s *Subscriber) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Close closes all topic channels and stops the server.
func (s *Subscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, handler := range s.topics {
		handler.cancel()
	}

	if s.server != nil {
		return s.server.Close()
	}
	return nil
}
