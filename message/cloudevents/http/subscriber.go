package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/fxsml/gopipe/message"
	ce "github.com/fxsml/gopipe/message/cloudevents"
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
//	sub := cehttp.NewSubscriber(cfg)
//
//	mux := http.NewServeMux()
//	mux.Handle("/events", sub)
//
//	ctx, cancel := context.WithCancel(context.Background())
//	ch, _ := sub.Subscribe(ctx)
//
//	go http.ListenAndServe(":8080", mux)
//
//	for msg := range ch {
//	    process(msg)
//	    msg.Ack()
//	}
//
//	cancel() // Stops accepting HTTP requests
type Subscriber struct {
	mu         sync.RWMutex
	ch         chan *message.RawMessage
	done       chan struct{}
	wg         sync.WaitGroup
	subscribed bool
	cfg        SubscriberConfig
}

// NewSubscriber creates an HTTP CloudEvents subscriber.
func NewSubscriber(cfg SubscriberConfig) *Subscriber {
	return &Subscriber{
		cfg: cfg.parse(),
	}
}

// Subscribe starts accepting messages and returns the channel to receive them.
// The context controls the subscriber lifecycle - when cancelled, HTTP requests
// return 503 and the channel is closed.
//
// Subscribe can only be called once. Multiple consumers can read from the
// returned channel concurrently (competing consumers pattern).
func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *message.RawMessage, error) {
	s.mu.Lock()
	if s.subscribed {
		s.mu.Unlock()
		return nil, errors.New("already subscribed")
	}
	s.ch = make(chan *message.RawMessage, s.cfg.BufferSize)
	s.done = make(chan struct{})
	s.subscribed = true
	s.mu.Unlock()

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		close(s.done)  // Signal shutdown to all handlers
		s.wg.Wait()    // Wait for in-flight requests
		close(s.ch)    // Safe to close now
	}()

	return s.ch, nil
}

// ServeHTTP implements http.Handler.
func (s *Subscriber) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if subscribed
	s.mu.RLock()
	if !s.subscribed {
		s.mu.RUnlock()
		http.Error(w, "no subscriber", http.StatusServiceUnavailable)
		return
	}
	s.mu.RUnlock()

	// Track this request for graceful shutdown
	s.wg.Add(1)
	defer s.wg.Done()

	// Fast-fail if subscriber context was cancelled
	select {
	case <-s.done:
		http.Error(w, "subscriber closed", http.StatusServiceUnavailable)
		return
	default:
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse using SDK (handles binary + structured + batch)
	var events []cloudevents.Event
	if cehttp.IsHTTPBatch(r.Header) {
		var err error
		events, err = cehttp.NewEventsFromHTTPRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		event, err := cehttp.NewEventFromHTTPRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		events = []cloudevents.Event{*event}
	}

	if len(events) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Track acks for all messages
	done := make(chan error, len(events))

	for i := range events {
		acking := message.NewAcking(
			func() { done <- nil },
			func(e error) { done <- e },
		)
		msg, err := ce.FromCloudEvent(&events[i], acking)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		select {
		case s.ch <- msg:
		case <-s.done:
			http.Error(w, "subscriber closed", http.StatusServiceUnavailable)
			return
		case <-r.Context().Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
	}

	// Wait for all acks
	var firstErr error
	timeout := time.NewTimer(s.cfg.AckTimeout)
	defer timeout.Stop()

	for range events {
		select {
		case err := <-done:
			if err != nil && firstErr == nil {
				firstErr = err
			}
		case <-s.done:
			http.Error(w, "subscriber closed", http.StatusServiceUnavailable)
			return
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
