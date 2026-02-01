package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/fxsml/gopipe/message"
	ce "github.com/fxsml/gopipe/message/cloudevents"
	"github.com/fxsml/gopipe/pipe"
)

// Content types for CloudEvents HTTP protocol binding.
const (
	ContentTypeCloudEventsJSON      = "application/cloudevents+json"
	ContentTypeCloudEventsBatchJSON = "application/cloudevents-batch+json"
)

// PublisherConfig configures an HTTP CloudEvents Publisher.
type PublisherConfig struct {
	// TargetURL is the full URL for sending events.
	TargetURL string

	// Client is the HTTP client to use (default: http.DefaultClient).
	Client *http.Client

	// Concurrency is the number of concurrent workers for Publish (default: 1).
	Concurrency int

	// Headers are additional HTTP headers to include in requests.
	Headers http.Header

	// StructuredMode uses structured content mode (metadata in JSON body).
	// Default is binary mode (metadata in Ce-* headers, more efficient).
	StructuredMode bool

	// BatchSize is the maximum messages per batch (default: 1).
	// Set to higher values for throughput (e.g., 10-100).
	BatchSize int

	// BatchDuration is the maximum time to wait before flushing a batch.
	BatchDuration time.Duration

	// ErrorHandler is called on send errors.
	ErrorHandler func(msg *message.RawMessage, err error)

	// Logger for structured logging.
	Logger message.Logger
}

func (c PublisherConfig) parse() PublisherConfig {
	if c.Client == nil {
		c.Client = http.DefaultClient
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	// Always use batch mode internally for consistent middleware support.
	// BatchSize=0 means no batching, which we treat as BatchSize=1.
	if c.BatchSize <= 0 {
		c.BatchSize = 1
	}
	if c.BatchDuration <= 0 {
		c.BatchDuration = time.Second
	}
	return c
}

// Publisher sends CloudEvents over HTTP to a target URL.
//
// Usage:
//
//	// Single mode (default, binary content mode)
//	pub := cehttp.NewPublisher(cehttp.PublisherConfig{
//	    TargetURL: "http://host/events/orders",
//	})
//	pub.Send(ctx, msg)              // Synchronous single
//	pub.Publish(ctx, ch)            // Channel-based, sends individually
//
//	// Batch mode
//	pub := cehttp.NewPublisher(cehttp.PublisherConfig{
//	    TargetURL:     "http://host/events/orders",
//	    BatchSize:     10,
//	    BatchDuration: 100 * time.Millisecond,
//	})
//	pub.SendBatch(ctx, msgs)        // Synchronous batch
//	pub.Publish(ctx, ch)            // Channel-based, batches automatically
type Publisher struct {
	cfg    PublisherConfig
	logger message.Logger

	mu      sync.Mutex
	started bool
}

// NewPublisher creates an HTTP CloudEvents publisher for the target URL.
func NewPublisher(cfg PublisherConfig) *Publisher {
	cfg = cfg.parse()
	return &Publisher{
		cfg:    cfg,
		logger: cfg.Logger,
	}
}

// Send sends a single CloudEvent synchronously.
// Uses binary mode by default, or structured mode if configured.
// Calls msg.Ack() on success (HTTP 2xx), msg.Nack(err) on failure.
func (p *Publisher) Send(ctx context.Context, msg *message.RawMessage) error {
	event, err := ce.ToCloudEvent(msg)
	if err != nil {
		msg.Nack(err)
		return fmt.Errorf("converting to CloudEvent: %w", err)
	}

	reqCtx := ctx
	if p.cfg.StructuredMode {
		reqCtx = binding.WithForceStructured(ctx)
	}

	req, err := cehttp.NewHTTPRequestFromEvent(reqCtx, p.cfg.TargetURL, *event)
	if err != nil {
		msg.Nack(err)
		return fmt.Errorf("creating request: %w", err)
	}

	for k, v := range p.cfg.Headers {
		req.Header[k] = v
	}

	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		msg.Nack(err)
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		msg.Ack()
		return nil
	}

	err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	msg.Nack(err)
	return err
}

// SendBatch sends multiple CloudEvents as a batch synchronously.
// Uses CloudEvents batch format (application/cloudevents-batch+json).
// All messages are acked on success, nacked on failure.
func (p *Publisher) SendBatch(ctx context.Context, msgs []*message.RawMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	events := make([]cloudevents.Event, len(msgs))
	for i, msg := range msgs {
		event, err := ce.ToCloudEvent(msg)
		if err != nil {
			for _, m := range msgs {
				m.Nack(err)
			}
			return fmt.Errorf("converting to CloudEvent: %w", err)
		}
		events[i] = *event
	}

	req, err := cehttp.NewHTTPRequestFromEvents(ctx, p.cfg.TargetURL, events)
	if err != nil {
		for _, msg := range msgs {
			msg.Nack(err)
		}
		return fmt.Errorf("creating request: %w", err)
	}

	for k, v := range p.cfg.Headers {
		req.Header[k] = v
	}

	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		for _, msg := range msgs {
			msg.Nack(err)
		}
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		for _, msg := range msgs {
			msg.Ack()
		}
		return nil
	}

	err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	for _, msg := range msgs {
		msg.Nack(err)
	}
	return err
}

// Publish consumes messages from a channel and sends them via HTTP.
// Uses batch mode internally for consistent middleware support.
// When BatchSize=1 (default), messages are sent individually using Send for efficiency.
// Returns a done channel that closes when all messages are sent.
func (p *Publisher) Publish(ctx context.Context, in <-chan *message.RawMessage) (<-chan struct{}, error) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil, pipe.ErrAlreadyStarted
	}
	p.started = true
	p.mu.Unlock()

	return p.publishBatch(ctx, in)
}

// publishBatch batches messages and sends as CloudEvents batch format.
// When batch size is 1, uses Send for efficiency (binary mode, less overhead).
func (p *Publisher) publishBatch(ctx context.Context, in <-chan *message.RawMessage) (<-chan struct{}, error) {
	batchPipe := pipe.NewBatchPipe(func(ctx context.Context, batch []*message.RawMessage) ([]struct{}, error) {
		var err error
		if len(batch) == 1 {
			// Use Send for single messages (more efficient binary mode)
			err = p.Send(ctx, batch[0])
		} else {
			err = p.SendBatch(ctx, batch)
		}
		return nil, err
	}, pipe.BatchConfig{
		MaxSize:     p.cfg.BatchSize,
		MaxDuration: p.cfg.BatchDuration,
		Config: pipe.Config{
			Concurrency: p.cfg.Concurrency,
			ErrorHandler: func(in any, err error) {
				batch, _ := in.([]*message.RawMessage)
				p.logger.Error("Batch send failed",
					"component", "http-publisher",
					"error", err,
					"url", p.cfg.TargetURL,
					"batch_size", len(batch))
				if p.cfg.ErrorHandler != nil {
					for _, msg := range batch {
						p.cfg.ErrorHandler(msg, err)
					}
				}
			},
		},
	})

	return batchPipe.Pipe(ctx, in)
}
