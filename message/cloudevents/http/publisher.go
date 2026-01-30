package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

const (
	// ContentTypeCloudEventsJSON is the content type for structured CloudEvents.
	ContentTypeCloudEventsJSON = "application/cloudevents+json"

	// ContentTypeCloudEventsBatchJSON is the content type for CloudEvents batch.
	ContentTypeCloudEventsBatchJSON = "application/cloudevents-batch+json"
)

// PublisherConfig configures an HTTP CloudEvents Publisher.
type PublisherConfig struct {
	// Client is the HTTP client to use (default: http.DefaultClient).
	Client *http.Client

	// TargetURL is the base URL for sending events.
	// Topic paths are appended: "http://host/events" + "/orders" → "http://host/events/orders".
	TargetURL string

	// Concurrency is the number of concurrent HTTP requests for stream/batch (default: 1).
	Concurrency int

	// Headers are additional HTTP headers to include in requests.
	Headers http.Header

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
	return c
}

// BatchConfig configures batch publishing behavior.
type BatchConfig struct {
	// MaxSize is the maximum batch size (default: 100).
	MaxSize int

	// MaxDuration is the maximum time to wait before flushing (default: 1s).
	MaxDuration time.Duration

	// Concurrency is the number of concurrent batch sends (default: 1).
	Concurrency int
}

func (c BatchConfig) parse() BatchConfig {
	if c.MaxSize <= 0 {
		c.MaxSize = 100
	}
	if c.MaxDuration <= 0 {
		c.MaxDuration = time.Second
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	return c
}

// Publisher sends CloudEvents over HTTP.
type Publisher struct {
	cfg    PublisherConfig
	logger message.Logger

	mu          sync.Mutex
	streamPipe  *pipe.ProcessPipe[*message.RawMessage, struct{}]
	streamTopic string
	batchPipe   *pipe.BatchPipe[*message.RawMessage, struct{}]
	batchTopic  string
}

// NewPublisher creates an HTTP CloudEvents publisher.
func NewPublisher(cfg PublisherConfig) *Publisher {
	cfg = cfg.parse()
	return &Publisher{
		cfg:    cfg,
		logger: cfg.Logger,
	}
}

// Publish sends a single CloudEvent to the target URL + topic.
// Calls msg.Ack() on success (HTTP 2xx), msg.Nack(err) on failure.
func (p *Publisher) Publish(ctx context.Context, topic string, msg *message.RawMessage) error {
	url := p.cfg.TargetURL + "/" + topic

	body, err := msg.MarshalJSON()
	if err != nil {
		msg.Nack(err)
		return fmt.Errorf("marshaling event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		msg.Nack(err)
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", ContentTypeCloudEventsJSON)
	for k, v := range p.cfg.Headers {
		req.Header[k] = v
	}

	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		msg.Nack(err)
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Drain body to enable connection reuse
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		msg.Ack()
		return nil
	}

	err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	msg.Nack(err)
	return err
}

// PublishStream consumes messages from a channel and sends them via HTTP.
// Uses SinkPipe internally for concurrent sending.
// Returns a done channel that closes when all messages are sent.
func (p *Publisher) PublishStream(
	ctx context.Context,
	topic string,
	in <-chan *message.RawMessage,
) (<-chan struct{}, error) {
	p.mu.Lock()
	if p.streamPipe != nil {
		p.mu.Unlock()
		return nil, pipe.ErrAlreadyStarted
	}

	p.streamTopic = topic
	p.streamPipe = pipe.NewSinkPipe(p.sendSingle, pipe.Config{
		Concurrency: p.cfg.Concurrency,
		ErrorHandler: func(in any, err error) {
			raw, _ := in.(*message.RawMessage)
			p.logger.Error("Message send failed",
				"component", "http-publisher",
				"error", err,
				"topic", topic,
				"attributes", raw.Attributes)
			if p.cfg.ErrorHandler != nil {
				p.cfg.ErrorHandler(raw, err)
			}
		},
	})
	p.mu.Unlock()

	return p.streamPipe.Pipe(ctx, in)
}

// sendSingle sends a single message (used by PublishStream).
func (p *Publisher) sendSingle(ctx context.Context, msg *message.RawMessage) error {
	return p.Publish(ctx, p.streamTopic, msg)
}

// PublishBatch consumes messages, batches them, and sends as CloudEvents batch format.
// Uses BatchPipe internally. Batch format is JSON array per CloudEvents spec.
// Acknowledgment: shared acking - batch acks when HTTP succeeds, nacks on failure.
func (p *Publisher) PublishBatch(
	ctx context.Context,
	topic string,
	in <-chan *message.RawMessage,
	cfg BatchConfig,
) (<-chan struct{}, error) {
	p.mu.Lock()
	if p.batchPipe != nil {
		p.mu.Unlock()
		return nil, pipe.ErrAlreadyStarted
	}

	cfg = cfg.parse()
	p.batchTopic = topic

	p.batchPipe = pipe.NewBatchPipe(p.sendBatch, pipe.BatchConfig{
		MaxSize:     cfg.MaxSize,
		MaxDuration: cfg.MaxDuration,
		Config: pipe.Config{
			Concurrency: cfg.Concurrency,
			ErrorHandler: func(in any, err error) {
				batch, _ := in.([]*message.RawMessage)
				p.logger.Error("Batch send failed",
					"component", "http-publisher",
					"error", err,
					"topic", topic,
					"batch_size", len(batch))
				if p.cfg.ErrorHandler != nil {
					for _, msg := range batch {
						p.cfg.ErrorHandler(msg, err)
					}
				}
			},
		},
	})
	p.mu.Unlock()

	return p.batchPipe.Pipe(ctx, in)
}

// sendBatch sends a batch of messages.
func (p *Publisher) sendBatch(ctx context.Context, batch []*message.RawMessage) ([]struct{}, error) {
	if len(batch) == 0 {
		return nil, nil
	}

	url := p.cfg.TargetURL + "/" + p.batchTopic

	body, err := MarshalBatch(batch)
	if err != nil {
		for _, msg := range batch {
			msg.Nack(err)
		}
		return nil, fmt.Errorf("marshaling batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		for _, msg := range batch {
			msg.Nack(err)
		}
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", ContentTypeCloudEventsBatchJSON)
	for k, v := range p.cfg.Headers {
		req.Header[k] = v
	}

	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		for _, msg := range batch {
			msg.Nack(err)
		}
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Drain body to enable connection reuse
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		for _, msg := range batch {
			msg.Ack()
		}
		return nil, nil
	}

	err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	for _, msg := range batch {
		msg.Nack(err)
	}
	return nil, err
}

// Use adds middleware to the publisher's stream pipe.
// Must be called before PublishStream.
func (p *Publisher) Use(mw ...middleware.Middleware[*message.RawMessage, struct{}]) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamPipe == nil {
		// Create a placeholder pipe for middleware
		p.streamPipe = pipe.NewSinkPipe(p.sendSingle, pipe.Config{
			Concurrency: p.cfg.Concurrency,
		})
	}

	return p.streamPipe.Use(mw...)
}

// UseBatch adds middleware to the publisher's batch pipe.
// Must be called before PublishBatch.
func (p *Publisher) UseBatch(mw ...middleware.Middleware[[]*message.RawMessage, struct{}]) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.batchPipe == nil {
		// Create a placeholder pipe for middleware
		p.batchPipe = pipe.NewBatchPipe(p.sendBatch, pipe.BatchConfig{})
	}

	return p.batchPipe.Use(mw...)
}
