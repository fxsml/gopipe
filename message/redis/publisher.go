package redis

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	goredis "github.com/redis/go-redis/v9"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// PublisherConfig configures a Redis Streams Publisher.
type PublisherConfig struct {
	// Stream is the destination Redis Stream (required).
	Stream string
	// MaxLen caps the stream length via XADD MAXLEN.
	// 0 means no limit (default).
	MaxLen int64
	// Approx uses approximate trimming (MAXLEN ~) for O(1) cost.
	// Only applies when MaxLen > 0. Default: true.
	Approx *bool

	// Marshal converts a RawMessage to stream entry field-value pairs.
	// Default: DefaultMarshal (JSON fields).
	Marshal MarshalFunc

	// Concurrency is the number of concurrent send goroutines (default: 1).
	Concurrency int

	// ErrorHandler is called on marshal/send errors.
	ErrorHandler func(msg *message.RawMessage, err error)
	// Logger for structured logging (default: slog.Default()).
	Logger message.Logger
}

func (c PublisherConfig) parse() PublisherConfig {
	if c.Marshal == nil {
		c.Marshal = DefaultMarshal
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	if c.Approx == nil {
		t := true
		c.Approx = &t
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Publisher sends messages to a Redis Stream via XADD.
// Backed by a SinkPipe for lifecycle and concurrency management.
type Publisher struct {
	client goredis.UniversalClient
	cfg    PublisherConfig
	logger message.Logger
	sink   *pipe.ProcessPipe[*message.RawMessage, struct{}]

	mu      sync.Mutex
	started bool
}

// NewPublisher creates a new Redis Streams publisher.
func NewPublisher(client goredis.UniversalClient, cfg PublisherConfig) *Publisher {
	cfg = cfg.parse()

	p := &Publisher{
		client: client,
		cfg:    cfg,
		logger: cfg.Logger,
	}

	p.sink = pipe.NewSinkPipe(p.send, pipe.Config{
		Concurrency: cfg.Concurrency,
		ErrorHandler: func(in any, err error) {
			raw, _ := in.(*message.RawMessage)
			p.logger.Error("Redis stream send failed",
				"component", "redis-publisher",
				"stream", cfg.Stream,
				"error", err)
			if cfg.ErrorHandler != nil {
				cfg.ErrorHandler(raw, err)
			}
		},
	})

	return p
}

// Use adds middleware to the publisher's internal pipe.
// Must be called before Publish.
func (p *Publisher) Use(mw ...middleware.Middleware[*message.RawMessage, struct{}]) error {
	return p.sink.Use(mw...)
}

// Publish starts consuming messages from the channel and sending them to the
// Redis Stream. Returns a done channel that closes when publishing completes.
//
// For each message:
//   - Successful XADD: calls msg.Ack()
//   - Failed XADD: calls msg.Nack(err)
//
// Returns ErrAlreadyStarted if Publish has already been called.
func (p *Publisher) Publish(ctx context.Context, ch <-chan *message.RawMessage) (<-chan struct{}, error) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil, pipe.ErrAlreadyStarted
	}
	p.started = true
	p.mu.Unlock()

	return p.sink.Pipe(ctx, ch)
}

func (p *Publisher) send(ctx context.Context, raw *message.RawMessage) error {
	values, err := p.cfg.Marshal(raw)
	if err != nil {
		raw.Nack(err)
		return fmt.Errorf("marshal: %w", err)
	}

	args := &goredis.XAddArgs{
		Stream: p.cfg.Stream,
		Values: values,
	}
	if p.cfg.MaxLen > 0 {
		args.MaxLen = p.cfg.MaxLen
		args.Approx = *p.cfg.Approx
	}

	_, err = p.client.XAdd(ctx, args).Result()
	if err != nil {
		raw.Nack(err)
		return err
	}

	raw.Ack()
	return nil
}
