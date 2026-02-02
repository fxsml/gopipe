package message

import (
	"context"
	"sync"
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
	ackFn            func()
	nackFn           func(error)
	settled          bool
	nackErr          error
	ackCount         int
	expectedAckCount int
	doneCh           chan struct{}
}

// NewAcking creates an Acking for a single message.
// Returns nil if either callback is nil.
//
// The ack and nack callbacks must not panic. If they do, the panic will
// propagate after cleanup (the done channel will still be closed to prevent
// resource leaks, but the program will crash).
func NewAcking(ack func(), nack func(error)) *Acking {
	if ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ackFn:            ack,
		nackFn:           nack,
		expectedAckCount: 1,
		doneCh:           make(chan struct{}),
	}
}

// NewSharedAcking creates an Acking shared across multiple messages.
// The ack callback is invoked after expectedCount Ack() calls.
// If any message nacks, all sibling messages' done channels are closed.
// Returns nil if expectedCount <= 0 or if either callback is nil.
//
// The ack and nack callbacks must not panic. If they do, the panic will
// propagate after cleanup (the done channel will still be closed to prevent
// resource leaks, but the program will crash).
func NewSharedAcking(ack func(), nack func(error), expectedCount int) *Acking {
	if expectedCount <= 0 || ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ackFn:            ack,
		nackFn:           nack,
		expectedAckCount: expectedCount,
		doneCh:           make(chan struct{}),
	}
}

// isSettled returns true if the acking has been settled (acked or nacked).
// Thread-safe.
func (a *Acking) isSettled() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settled
}

// err returns the nack error, or nil if pending or acked.
// Thread-safe.
func (a *Acking) err() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.nackErr
}

// done returns the done channel, or nil if no acking.
func (a *Acking) done() <-chan struct{} {
	if a == nil {
		return nil
	}
	return a.doneCh
}

// ack acknowledges successful processing.
func (a *Acking) ack() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()

	if a.settled {
		wasAcked := a.nackErr == nil
		a.mu.Unlock()
		return wasAcked
	}

	a.ackCount++
	if a.ackCount < a.expectedAckCount {
		a.mu.Unlock()
		return true
	}

	// Capture callback and done channel before releasing lock
	ackFn := a.ackFn
	done := a.doneCh
	a.settled = true
	a.mu.Unlock()

	// Ensure done channel closes even if callback panics
	defer close(done)

	// Call callback outside mutex to prevent deadlock
	ackFn()
	return true
}

// nack negatively acknowledges due to a processing error.
func (a *Acking) nack(err error) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()

	if a.settled {
		wasNacked := a.nackErr != nil
		a.mu.Unlock()
		return wasNacked
	}

	// Capture callback and done channel before releasing lock
	nackFn := a.nackFn
	done := a.doneCh
	a.settled = true
	a.nackErr = err
	a.mu.Unlock()

	// Ensure done channel closes even if callback panics
	defer close(done)

	// Call callback outside mutex to prevent deadlock
	nackFn(err)
	return true
}

// manualAckMiddleware returns middleware that lets the handler manage acking.
// Only auto-nacks on error if the message wasn't already acked/nacked.
func manualAckMiddleware() Middleware {
	return func(next ProcessFunc) ProcessFunc {
		return func(ctx context.Context, msg *Message) ([]*Message, error) {
			outputs, err := next(ctx, msg)
			if err != nil {
				msg.Nack(err) // Idempotent - no-op if already settled
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
			if msg.acking.isSettled() {
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
