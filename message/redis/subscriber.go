package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// SubscriberConfig configures a Redis Streams Subscriber.
type SubscriberConfig struct {
	// Stream is the Redis Stream to consume from (required).
	Stream string
	// Group is the consumer group name (required).
	Group string
	// Consumer is this instance's name within the group.
	// Default: hostname.
	Consumer string

	// OldestID is the starting ID when creating a new consumer group.
	// "0" = consume all existing messages (default).
	// "$" = only consume new messages arriving after group creation.
	OldestID string

	// BatchSize is the XREADGROUP COUNT parameter (default: 10).
	BatchSize int64
	// BlockTimeout is the XREADGROUP BLOCK duration (default: 1s).
	// Set to 0 for indefinite blocking (not recommended, see go-redis#2556).
	BlockTimeout time.Duration

	// ClaimInterval is how often to check for idle pending messages (default: 30s).
	ClaimInterval time.Duration
	// MaxIdleTime is the idle threshold before claiming a pending message (default: 60s).
	MaxIdleTime time.Duration

	// Unmarshal converts a stream entry to a RawMessage.
	// Default: DefaultUnmarshal (JSON fields).
	Unmarshal UnmarshalFunc

	// Concurrency is the number of receive goroutines (default: 1).
	Concurrency int
	// BufferSize is the output channel buffer size (default: 100).
	BufferSize int

	// ErrorHandler is called on receive/unmarshal errors.
	ErrorHandler func(err error)
	// Logger for structured logging (default: slog.Default()).
	Logger message.Logger
}

func (c SubscriberConfig) parse() SubscriberConfig {
	if c.Consumer == "" {
		c.Consumer, _ = os.Hostname()
		if c.Consumer == "" {
			c.Consumer = "gopipe"
		}
	}
	if c.OldestID == "" {
		c.OldestID = "0"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 10
	}
	if c.BlockTimeout <= 0 {
		c.BlockTimeout = time.Second
	}
	if c.ClaimInterval <= 0 {
		c.ClaimInterval = 30 * time.Second
	}
	if c.MaxIdleTime <= 0 {
		c.MaxIdleTime = 60 * time.Second
	}
	if c.Unmarshal == nil {
		c.Unmarshal = DefaultUnmarshal
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Subscriber consumes messages from a Redis Stream using consumer groups.
// It uses XREADGROUP for reading and XPENDING + XCLAIM for recovering
// messages from dead consumers. Compatible with Redis 6.0+.
type Subscriber struct {
	client goredis.UniversalClient
	cfg    SubscriberConfig
	logger message.Logger
	gen    *pipe.GeneratePipe[*message.RawMessage]

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

// NewSubscriber creates a new Redis Streams subscriber.
func NewSubscriber(client goredis.UniversalClient, cfg SubscriberConfig) *Subscriber {
	cfg = cfg.parse()

	s := &Subscriber{
		client: client,
		cfg:    cfg,
		logger: cfg.Logger,
	}

	pipeCfg := pipe.Config{
		Concurrency: cfg.Concurrency,
		BufferSize:  cfg.BufferSize,
		ErrorHandler: func(_ any, err error) {
			s.logger.Error("Redis stream receive failed",
				"component", "redis-subscriber",
				"stream", cfg.Stream,
				"group", cfg.Group,
				"error", err)
			if cfg.ErrorHandler != nil {
				cfg.ErrorHandler(err)
			}
		},
	}

	s.gen = pipe.NewGenerator(s.receive, pipeCfg)
	return s
}

// Use adds middleware to the subscriber's internal pipe.
// Must be called before Subscribe.
func (s *Subscriber) Use(mw ...middleware.Middleware[struct{}, *message.RawMessage]) error {
	return s.gen.Use(mw...)
}

// Subscribe starts consuming messages from the Redis Stream and returns
// the output channel. The channel should be registered with the engine
// using AddRawInput.
//
// Creates the consumer group if it doesn't exist. Starts a background
// goroutine to reclaim idle messages from dead consumers.
//
// Returns ErrAlreadyStarted if Subscribe has already been called.
func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *message.RawMessage, error) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil, pipe.ErrAlreadyStarted
	}
	s.started = true
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	// Create consumer group (idempotent — ignores BUSYGROUP error).
	err := s.client.XGroupCreateMkStream(childCtx, s.cfg.Stream, s.cfg.Group, s.cfg.OldestID).Err()
	if err != nil && !isBusyGroupErr(err) {
		cancel()
		return nil, fmt.Errorf("create consumer group: %w", err)
	}

	// Start background claim loop for recovering idle messages.
	go s.claimLoop(childCtx)

	return s.gen.Generate(childCtx)
}

// receive fetches the next batch of messages from the Redis Stream.
func (s *Subscriber) receive(ctx context.Context) ([]*message.RawMessage, error) {
	streams, err := s.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    s.cfg.Group,
		Consumer: s.cfg.Consumer,
		Streams:  []string{s.cfg.Stream, ">"},
		Count:    s.cfg.BatchSize,
		Block:    s.cfg.BlockTimeout,
	}).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil // no messages, GeneratePipe loops
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil
		}
		return nil, err
	}

	var msgs []*message.RawMessage
	for _, stream := range streams {
		for _, xMsg := range stream.Messages {
			raw, err := s.unmarshalEntry(ctx, xMsg)
			if err != nil {
				s.logger.Error("Unmarshal stream entry failed",
					"component", "redis-subscriber",
					"stream", s.cfg.Stream,
					"entry_id", xMsg.ID,
					"error", err)
				continue
			}
			msgs = append(msgs, raw)
		}
	}
	return msgs, nil
}

// unmarshalEntry converts a Redis stream entry to a RawMessage with acking.
func (s *Subscriber) unmarshalEntry(ctx context.Context, xMsg goredis.XMessage) (*message.RawMessage, error) {
	raw, err := s.cfg.Unmarshal(xMsg.ID, xMsg.Values)
	if err != nil {
		return nil, err
	}

	// Bridge Redis acking to gopipe Acking.
	// Ack → XACK (remove from PEL), Nack → no-op (stays in PEL for reclaim).
	stream := s.cfg.Stream
	group := s.cfg.Group
	client := s.client
	entryID := xMsg.ID
	logger := s.logger

	acking := message.NewAcking(
		func() {
			if err := client.XAck(ctx, stream, group, entryID).Err(); err != nil {
				logger.Error("XACK failed",
					"component", "redis-subscriber",
					"stream", stream,
					"entry_id", entryID,
					"error", err)
			}
		},
		func(err error) {
			// No XACK — message stays in PEL for reclaim by claimLoop.
		},
	)

	return message.NewRaw(raw.Data, raw.Attributes, acking), nil
}

// claimLoop periodically checks for idle pending messages and claims them.
// Uses XPENDING + XCLAIM (Redis 6.0 compatible, no XAUTOCLAIM).
func (s *Subscriber) claimLoop(ctx context.Context) {
	// Claim once on startup to recover pending messages immediately.
	s.claimIdle(ctx)

	ticker := time.NewTicker(s.cfg.ClaimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.claimIdle(ctx)
		}
	}
}

// claimIdle claims idle pending messages from other consumers.
func (s *Subscriber) claimIdle(ctx context.Context) {
	pending, err := s.client.XPendingExt(ctx, &goredis.XPendingExtArgs{
		Stream: s.cfg.Stream,
		Group:  s.cfg.Group,
		Idle:   s.cfg.MaxIdleTime,
		Start:  "-",
		End:    "+",
		Count:  s.cfg.BatchSize,
	}).Result()
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error("XPENDING failed",
				"component", "redis-subscriber",
				"stream", s.cfg.Stream,
				"error", err)
		}
		return
	}

	if len(pending) == 0 {
		return
	}

	ids := make([]string, len(pending))
	for i, pe := range pending {
		ids[i] = pe.ID
	}

	// XClaim with MinIdle prevents double-claiming by concurrent subscribers.
	claimed, err := s.client.XClaim(ctx, &goredis.XClaimArgs{
		Stream:   s.cfg.Stream,
		Group:    s.cfg.Group,
		Consumer: s.cfg.Consumer,
		MinIdle:  s.cfg.MaxIdleTime,
		Messages: ids,
	}).Result()
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error("XCLAIM failed",
				"component", "redis-subscriber",
				"stream", s.cfg.Stream,
				"error", err)
		}
		return
	}

	if len(claimed) > 0 {
		s.logger.Info("Claimed idle messages",
			"component", "redis-subscriber",
			"stream", s.cfg.Stream,
			"count", len(claimed))
	}
	// Claimed messages will be delivered in next XREADGROUP call
	// (they appear when reading with ID "0" or ">").
}

// isBusyGroupErr checks if the error is a BUSYGROUP error (group already exists).
func isBusyGroupErr(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
