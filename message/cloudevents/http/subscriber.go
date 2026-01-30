package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/fxsml/gopipe/message"
)

// SubscriberConfig configures an HTTP CloudEvents Subscriber.
type SubscriberConfig struct {
	// BufferSize is the channel buffer size (default: 100).
	BufferSize int

	// AckTimeout is the maximum time to wait for ack/nack (default: 30s).
	AckTimeout time.Duration
}

func (c SubscriberConfig) parse() SubscriberConfig {
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.AckTimeout <= 0 {
		c.AckTimeout = 30 * time.Second
	}
	return c
}

// Subscriber receives CloudEvents over HTTP and delivers to a channel.
// Implements http.Handler for use with standard library routing.
//
// Usage:
//
//	orders := http.NewSubscriber(cfg)
//	payments := http.NewSubscriber(cfg)
//
//	mux := http.NewServeMux()
//	mux.Handle("/events/orders", orders)
//	mux.Handle("/events/payments", payments)
//	http.ListenAndServe(":8080", mux)
type Subscriber struct {
	ch  chan *message.RawMessage
	cfg SubscriberConfig
}

// NewSubscriber creates an HTTP CloudEvents subscriber.
func NewSubscriber(cfg SubscriberConfig) *Subscriber {
	cfg = cfg.parse()
	return &Subscriber{
		ch:  make(chan *message.RawMessage, cfg.BufferSize),
		cfg: cfg,
	}
}

// C returns the channel for receiving messages.
func (s *Subscriber) C() <-chan *message.RawMessage {
	return s.ch
}

// Close closes the message channel.
func (s *Subscriber) Close() {
	close(s.ch)
}

// ServeHTTP implements http.Handler.
func (s *Subscriber) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse based on content type
	contentType := r.Header.Get("Content-Type")
	isBatch := strings.Contains(contentType, ContentTypeCloudEventsBatchJSON)

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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(msgs) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Track acks for all messages
	done := make(chan error, len(msgs))

	for _, msg := range msgs {
		acking := message.NewAcking(
			func() { done <- nil },
			func(e error) { done <- e },
		)
		msgWithAck := message.NewRaw(msg.Data, msg.Attributes, acking)

		select {
		case s.ch <- msgWithAck:
		case <-r.Context().Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
	}

	// Wait for all acks
	var firstErr error
	timeout := time.NewTimer(s.cfg.AckTimeout)
	defer timeout.Stop()

	for range msgs {
		select {
		case err := <-done:
			if err != nil && firstErr == nil {
				firstErr = err
			}
		case <-timeout.C:
			http.Error(w, "ack timeout", http.StatusGatewayTimeout)
			return
		case <-r.Context().Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
	}

	if firstErr != nil {
		http.Error(w, firstErr.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
