package cloudevents

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
)

// SubscriberConfig configures a Subscriber.
type SubscriberConfig struct {
	// Buffer is the output channel buffer size (default: 100).
	Buffer int
	// Concurrency is the number of receive goroutines (default: 1).
	Concurrency int
	// ErrorHandler is called on receive/conversion errors (default: no-op).
	ErrorHandler func(err error)
	// Logger is used for logging (default: slog.Default()).
	Logger message.Logger
}

// Subscriber wraps a CloudEvents protocol.Receiver as a gopipe input source.
// It receives binding.Messages, converts them to RawMessages, and bridges
// the CloudEvents Finish() acknowledgment to gopipe's Acking callbacks.
type Subscriber struct {
	receiver protocol.Receiver
	cfg      SubscriberConfig
	logger   message.Logger
	gen      *pipe.GeneratePipe[*message.RawMessage]

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

// NewSubscriber creates a new Subscriber wrapping the given receiver.
func NewSubscriber(receiver protocol.Receiver, cfg SubscriberConfig) *Subscriber {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	s := &Subscriber{
		receiver: receiver,
		cfg:      cfg,
		logger:   logger,
	}

	pipeCfg := pipe.Config{
		Concurrency: cfg.Concurrency,
		BufferSize:  cfg.Buffer,
		ErrorHandler: func(_ any, err error) {
			logger.Error("Subscriber error",
				"error", err)
			if cfg.ErrorHandler != nil {
				cfg.ErrorHandler(err)
			}
		},
	}
	if pipeCfg.Concurrency <= 0 {
		pipeCfg.Concurrency = 1
	}
	if pipeCfg.BufferSize <= 0 {
		pipeCfg.BufferSize = 100
	}

	s.gen = pipe.NewGenerator(s.receive, pipeCfg)
	return s
}

// Subscribe starts receiving messages from the CloudEvents receiver and returns
// the output channel. The channel should be registered with the engine using AddRawInput.
// Spawns goroutines (based on Concurrency) that run until the context is cancelled
// or the receiver returns EOF. The returned channel is closed when complete.
// Returns ErrAlreadyStarted if Subscribe has already been called.
func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *message.RawMessage, error) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil, pipe.ErrAlreadyStarted
	}
	s.started = true

	// Create child context so we can cancel on EOF
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	return s.gen.Generate(childCtx)
}

// receive fetches the next message from the CloudEvents receiver and converts it.
func (s *Subscriber) receive(ctx context.Context) ([]*message.RawMessage, error) {
	ceMsg, err := s.receiver.Receive(ctx)
	if err != nil {
		if err == io.EOF {
			// Signal completion by cancelling context
			s.mu.Lock()
			if s.cancel != nil {
				s.cancel()
			}
			s.mu.Unlock()
			return nil, nil
		}
		if ctx.Err() != nil {
			return nil, nil
		}
		return nil, err
	}

	// Convert binding.Message to cloudevents.Event
	event, err := binding.ToEvent(ctx, ceMsg)
	if err != nil {
		if finishErr := ceMsg.Finish(err); finishErr != nil {
			s.logger.Error("Finish error", "error", finishErr)
		}
		return nil, err
	}

	// Bridge CloudEvents Finish() to gopipe Acking
	msg := ceMsg
	logger := s.logger
	acking := message.NewAcking(
		func() {
			if err := msg.Finish(nil); err != nil {
				logger.Error("Ack finish error", "error", err)
			}
		},
		func(err error) {
			if finishErr := msg.Finish(err); finishErr != nil {
				logger.Error("Nack finish error", "error", finishErr)
			}
		},
	)

	attrs := extractAttributes(event)
	raw := message.NewRaw(event.Data(), attrs, acking)

	return []*message.RawMessage{raw}, nil
}
