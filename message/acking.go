package message

import (
	"context"
	"sync"
)

// AckState represents the current acknowledgment state.
type AckState int

const (
	// AckPending indicates the message has not been acked or nacked yet.
	AckPending AckState = iota
	// AckDone indicates the message was successfully acknowledged.
	AckDone
	// AckNacked indicates the message was negatively acknowledged.
	AckNacked
)

// AckStrategy determines how the router handles message acknowledgment.
type AckStrategy int

const (
	// AckOnSuccess automatically acks on successful processing
	// and nacks on error. This is the default.
	AckOnSuccess AckStrategy = iota

	// AckManual means the handler is responsible for acking/nacking.
	// Provides maximum control but requires explicit ack/nack calls.
	AckManual

	// AckForward forwards acknowledgment to output messages.
	// The input is acked only when ALL outputs are acked.
	// If ANY output nacks, the input is immediately nacked.
	// Useful for event sourcing where a command should only be
	// acked after all resulting events are processed.
	AckForward
)

// String implements fmt.Stringer.
func (s AckStrategy) String() string {
	switch s {
	case AckManual:
		return "manual"
	case AckForward:
		return "forward"
	default:
		return "on_success"
	}
}

// middleware returns the middleware for this ack strategy.
func (s AckStrategy) middleware() Middleware {
	switch s {
	case AckManual:
		return manualAckMiddleware()
	case AckForward:
		return forwardAckMiddleware()
	default:
		return autoAckMiddleware()
	}
}

// Acking coordinates acknowledgment across one or more messages.
// When expectedAckCount messages call Ack(), the ack callback is invoked.
// If any message calls Nack(), the nack callback is invoked immediately,
// and the done channel is closed.
// Acking is thread-safe and can be shared between multiple messages.
type Acking struct {
	mu               sync.Mutex
	ack              func()
	nack             func(error)
	ackState         AckState
	ackCount         int
	expectedAckCount int
	doneCh           chan struct{}
}

// NewAcking creates an Acking for a single message.
// Returns nil if either callback is nil.
func NewAcking(ack func(), nack func(error)) *Acking {
	if ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ack:              ack,
		nack:             nack,
		expectedAckCount: 1,
		doneCh:           make(chan struct{}),
	}
}

// NewSharedAcking creates an Acking shared across multiple messages.
// The ack callback is invoked after expectedCount Ack() calls.
// If any message nacks, all sibling messages' done channels are closed.
// Returns nil if expectedCount <= 0 or if either callback is nil.
func NewSharedAcking(ack func(), nack func(error), expectedCount int) *Acking {
	if expectedCount <= 0 || ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ack:              ack,
		nack:             nack,
		expectedAckCount: expectedCount,
		doneCh:           make(chan struct{}),
	}
}

// state returns the current acknowledgment state.
// Thread-safe.
func (a *Acking) state() AckState {
	if a == nil {
		return AckPending
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ackState
}

// manualAckMiddleware returns middleware that lets the handler manage acking.
// Only auto-nacks on error if the message wasn't already acked/nacked.
func manualAckMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil && msg.AckState() == AckPending {
				msg.Nack(err)
			}
			return outputs, err
		}
	}
}

// autoAckMiddleware returns middleware that acks on success, nacks on error.
func autoAckMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}
			msg.Ack()
			return outputs, nil
		}
	}
}

// forwardAckMiddleware returns middleware that forwards ack to output messages.
func forwardAckMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err)
				return nil, err
			}

			// No outputs - ack input immediately
			if len(outputs) == 0 {
				msg.Ack()
				return outputs, nil
			}

			// Check if input was already acked (handler acked manually)
			if msg.AckState() != AckPending {
				return outputs, nil
			}

			// Create shared acking: input acked when all outputs acked
			shared := NewSharedAcking(
				func() { msg.Ack() },
				func(e error) { msg.Nack(e) },
				len(outputs),
			)

			// Replace each output's acking with the shared acking
			for _, out := range outputs {
				out.acking = shared
			}

			return outputs, nil
		}
	}
}
