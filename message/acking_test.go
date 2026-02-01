package message

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAckingTableTests contains comprehensive table-driven tests for acking functionality.
func TestAckingTableTests(t *testing.T) {
	t.Run("SingleMessage", func(t *testing.T) {
		tests := []struct {
			name        string
			setup       func() (*Message, *int32, *int32, *error)
			action      func(*Message)
			wantAcked   int32
			wantNacked  int32
			wantState   AckState
			wantDone    bool
			wantAckRet  bool
			wantNackRet bool
		}{
			{
				name: "ack succeeds on first call",
				setup: func() (*Message, *int32, *int32, *error) {
					var ackCount, nackCount int32
					var nackErr error
					acking := NewAcking(
						func() { atomic.AddInt32(&ackCount, 1) },
						func(err error) { atomic.AddInt32(&nackCount, 1); nackErr = err },
					)
					msg := New("data", nil, acking)
					return msg, &ackCount, &nackCount, &nackErr
				},
				action:     func(m *Message) { m.Ack() },
				wantAcked:  1,
				wantNacked: 0,
				wantState:  AckDone,
				wantDone:   true,
			},
			{
				name: "nack succeeds on first call",
				setup: func() (*Message, *int32, *int32, *error) {
					var ackCount, nackCount int32
					var nackErr error
					acking := NewAcking(
						func() { atomic.AddInt32(&ackCount, 1) },
						func(err error) { atomic.AddInt32(&nackCount, 1); nackErr = err },
					)
					msg := New("data", nil, acking)
					return msg, &ackCount, &nackCount, &nackErr
				},
				action:     func(m *Message) { m.Nack(errors.New("test error")) },
				wantAcked:  0,
				wantNacked: 1,
				wantState:  AckNacked,
				wantDone:   true,
			},
			{
				name: "ack is idempotent - second call returns true but no callback",
				setup: func() (*Message, *int32, *int32, *error) {
					var ackCount, nackCount int32
					var nackErr error
					acking := NewAcking(
						func() { atomic.AddInt32(&ackCount, 1) },
						func(err error) { atomic.AddInt32(&nackCount, 1); nackErr = err },
					)
					msg := New("data", nil, acking)
					return msg, &ackCount, &nackCount, &nackErr
				},
				action: func(m *Message) {
					m.Ack()
					m.Ack()
					m.Ack()
				},
				wantAcked:  1,
				wantNacked: 0,
				wantState:  AckDone,
				wantDone:   true,
			},
			{
				name: "nack is idempotent - second call returns true but no callback",
				setup: func() (*Message, *int32, *int32, *error) {
					var ackCount, nackCount int32
					var nackErr error
					acking := NewAcking(
						func() { atomic.AddInt32(&ackCount, 1) },
						func(err error) { atomic.AddInt32(&nackCount, 1); nackErr = err },
					)
					msg := New("data", nil, acking)
					return msg, &ackCount, &nackCount, &nackErr
				},
				action: func(m *Message) {
					m.Nack(errors.New("first"))
					m.Nack(errors.New("second"))
				},
				wantAcked:  0,
				wantNacked: 1, // Only first nack callback invoked
				wantState:  AckNacked,
				wantDone:   true,
			},
			{
				name: "ack after nack returns false",
				setup: func() (*Message, *int32, *int32, *error) {
					var ackCount, nackCount int32
					var nackErr error
					acking := NewAcking(
						func() { atomic.AddInt32(&ackCount, 1) },
						func(err error) { atomic.AddInt32(&nackCount, 1); nackErr = err },
					)
					msg := New("data", nil, acking)
					return msg, &ackCount, &nackCount, &nackErr
				},
				action: func(m *Message) {
					m.Nack(errors.New("failed"))
					m.Ack() // Should return false and not invoke callback
				},
				wantAcked:  0,
				wantNacked: 1,
				wantState:  AckNacked,
				wantDone:   true,
			},
			{
				name: "nack after ack returns false",
				setup: func() (*Message, *int32, *int32, *error) {
					var ackCount, nackCount int32
					var nackErr error
					acking := NewAcking(
						func() { atomic.AddInt32(&ackCount, 1) },
						func(err error) { atomic.AddInt32(&nackCount, 1); nackErr = err },
					)
					msg := New("data", nil, acking)
					return msg, &ackCount, &nackCount, &nackErr
				},
				action: func(m *Message) {
					m.Ack()
					m.Nack(errors.New("failed")) // Should return false and not invoke callback
				},
				wantAcked:  1,
				wantNacked: 0,
				wantState:  AckDone,
				wantDone:   true,
			},
			{
				name: "nil acking - ack returns false",
				setup: func() (*Message, *int32, *int32, *error) {
					msg := New("data", nil, nil)
					return msg, new(int32), new(int32), new(error)
				},
				action:     func(m *Message) {},
				wantAcked:  0,
				wantNacked: 0,
				wantState:  AckPending, // nil acking returns AckPending
				wantDone:   false,      // No done channel
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				msg, ackCount, nackCount, _ := tt.setup()
				tt.action(msg)

				if got := atomic.LoadInt32(ackCount); got != tt.wantAcked {
					t.Errorf("ack callback count = %d, want %d", got, tt.wantAcked)
				}
				if got := atomic.LoadInt32(nackCount); got != tt.wantNacked {
					t.Errorf("nack callback count = %d, want %d", got, tt.wantNacked)
				}
				if got := msg.AckState(); got != tt.wantState {
					t.Errorf("AckState() = %v, want %v", got, tt.wantState)
				}

				// Check done channel
				done := msg.Done()
				if tt.wantDone {
					if done == nil {
						t.Error("Done() returned nil, expected channel")
					} else {
						select {
						case <-done:
							// OK - channel is closed
						default:
							t.Error("Done() channel not closed after settlement")
						}
					}
				} else if done != nil {
					select {
					case <-done:
						t.Error("Done() channel closed unexpectedly")
					default:
						// OK - channel not closed
					}
				}
			})
		}
	})

	t.Run("SharedAcking", func(t *testing.T) {
		tests := []struct {
			name       string
			count      int
			actions    []func([]*Message)
			wantAcked  int32
			wantNacked int32
			wantState  AckState
			wantDone   bool
		}{
			{
				name:  "all messages ack - callback invoked",
				count: 3,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
					func(msgs []*Message) { msgs[1].Ack() },
					func(msgs []*Message) { msgs[2].Ack() },
				},
				wantAcked:  1,
				wantNacked: 0,
				wantState:  AckDone,
				wantDone:   true,
			},
			{
				name:  "one nack blocks all acks",
				count: 3,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
					func(msgs []*Message) { msgs[1].Nack(errors.New("fail")) },
					func(msgs []*Message) { msgs[2].Ack() }, // Should return false
				},
				wantAcked:  0,
				wantNacked: 1,
				wantState:  AckNacked,
				wantDone:   true,
			},
			{
				name:  "partial acks - callback not invoked",
				count: 3,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
					func(msgs []*Message) { msgs[1].Ack() },
					// msgs[2] not acked
				},
				wantAcked:  0,
				wantNacked: 0,
				wantState:  AckPending,
				wantDone:   false,
			},
			{
				name:  "single message in shared acking",
				count: 1,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
				},
				wantAcked:  1,
				wantNacked: 0,
				wantState:  AckDone,
				wantDone:   true,
			},
			{
				name:  "nack first - all subsequent acks fail",
				count: 5,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Nack(errors.New("early fail")) },
					func(msgs []*Message) { msgs[1].Ack() },
					func(msgs []*Message) { msgs[2].Ack() },
					func(msgs []*Message) { msgs[3].Ack() },
					func(msgs []*Message) { msgs[4].Ack() },
				},
				wantAcked:  0,
				wantNacked: 1,
				wantState:  AckNacked,
				wantDone:   true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var ackCount, nackCount int32
				acking := NewSharedAcking(
					func() { atomic.AddInt32(&ackCount, 1) },
					func(error) { atomic.AddInt32(&nackCount, 1) },
					tt.count,
				)

				msgs := make([]*Message, tt.count)
				for i := range msgs {
					msgs[i] = New("data", nil, acking)
				}

				for _, action := range tt.actions {
					action(msgs)
				}

				if got := atomic.LoadInt32(&ackCount); got != tt.wantAcked {
					t.Errorf("ack callback count = %d, want %d", got, tt.wantAcked)
				}
				if got := atomic.LoadInt32(&nackCount); got != tt.wantNacked {
					t.Errorf("nack callback count = %d, want %d", got, tt.wantNacked)
				}
				if got := msgs[0].AckState(); got != tt.wantState {
					t.Errorf("AckState() = %v, want %v", got, tt.wantState)
				}

				done := msgs[0].Done()
				if tt.wantDone {
					select {
					case <-done:
						// OK
					default:
						t.Error("Done() channel not closed")
					}
				} else {
					select {
					case <-done:
						t.Error("Done() channel closed unexpectedly")
					default:
						// OK
					}
				}
			})
		}
	})

	t.Run("ContextBehavior", func(t *testing.T) {
		tests := []struct {
			name        string
			setup       func() (*Message, func())
			wantErr     error
			wantMessage bool
		}{
			{
				name: "context cancelled after ack",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Ack() }
				},
				wantErr:     context.Canceled,
				wantMessage: true,
			},
			{
				name: "context cancelled after nack",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Nack(errors.New("fail")) }
				},
				wantErr:     context.Canceled,
				wantMessage: true,
			},
			{
				name: "context not cancelled before settlement",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() {} // No action
				},
				wantErr:     nil,
				wantMessage: true,
			},
			{
				name: "nil acking returns background context",
				setup: func() (*Message, func()) {
					msg := New("data", nil, nil)
					return msg, func() {}
				},
				wantErr:     nil,
				wantMessage: false, // No message in background context
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				msg, action := tt.setup()
				ctx := msg.Context()

				// Verify context not cancelled before action
				if tt.wantErr == nil {
					if ctx.Err() != nil {
						t.Errorf("Context.Err() = %v before action, want nil", ctx.Err())
					}
				}

				action()

				// Verify context state after action
				if got := ctx.Err(); got != tt.wantErr {
					t.Errorf("Context.Err() = %v, want %v", got, tt.wantErr)
				}

				// Verify message in context
				msgFromCtx := MessageFromContext(ctx)
				if tt.wantMessage {
					if msgFromCtx != msg {
						t.Errorf("MessageFromContext() = %v, want %v", msgFromCtx, msg)
					}
				} else {
					if msgFromCtx != nil {
						t.Errorf("MessageFromContext() = %v, want nil", msgFromCtx)
					}
				}
			})
		}
	})

	t.Run("DoneChannelBehavior", func(t *testing.T) {
		tests := []struct {
			name       string
			setup      func() (*Message, func())
			wantClosed bool
		}{
			{
				name: "done closed on ack",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Ack() }
				},
				wantClosed: true,
			},
			{
				name: "done closed on nack",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Nack(errors.New("fail")) }
				},
				wantClosed: true,
			},
			{
				name: "done not closed before settlement",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() {}
				},
				wantClosed: false,
			},
			{
				name: "shared acking - done closed when all ack",
				setup: func() (*Message, func()) {
					acking := NewSharedAcking(func() {}, func(error) {}, 2)
					msg1 := New("data1", nil, acking)
					msg2 := New("data2", nil, acking)
					return msg1, func() {
						msg1.Ack()
						msg2.Ack()
					}
				},
				wantClosed: true,
			},
			{
				name: "shared acking - done closed on first nack",
				setup: func() (*Message, func()) {
					acking := NewSharedAcking(func() {}, func(error) {}, 3)
					msg1 := New("data1", nil, acking)
					msg2 := New("data2", nil, acking)
					return msg1, func() {
						msg1.Ack()
						msg2.Nack(errors.New("fail"))
						// msg3 never created or acked
					}
				},
				wantClosed: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				msg, action := tt.setup()
				done := msg.Done()

				// Verify not closed before action (if we expect it to close)
				if tt.wantClosed && done != nil {
					select {
					case <-done:
						t.Error("Done() closed before action")
					default:
						// OK
					}
				}

				action()

				if done == nil {
					if tt.wantClosed {
						t.Error("Done() returned nil, expected channel")
					}
					return
				}

				select {
				case <-done:
					if !tt.wantClosed {
						t.Error("Done() closed unexpectedly")
					}
				default:
					if tt.wantClosed {
						t.Error("Done() not closed after settlement")
					}
				}
			})
		}
	})
}

// TestAckingConcurrency tests thread-safety of acking operations.
func TestAckingConcurrency(t *testing.T) {
	t.Run("concurrent acks on shared acking", func(t *testing.T) {
		const count = 100
		var ackCount int32
		acking := NewSharedAcking(
			func() { atomic.AddInt32(&ackCount, 1) },
			func(error) {},
			count,
		)

		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				msg := New("data", nil, acking)
				msg.Ack()
			}()
		}
		wg.Wait()

		if got := atomic.LoadInt32(&ackCount); got != 1 {
			t.Errorf("ack callback count = %d, want 1", got)
		}
	})

	t.Run("concurrent ack and nack race", func(t *testing.T) {
		// This tests that either ack or nack wins, but not both
		for i := 0; i < 100; i++ {
			var ackCount, nackCount int32
			acking := NewAcking(
				func() { atomic.AddInt32(&ackCount, 1) },
				func(error) { atomic.AddInt32(&nackCount, 1) },
			)
			msg := New("data", nil, acking)

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				msg.Ack()
			}()
			go func() {
				defer wg.Done()
				msg.Nack(errors.New("fail"))
			}()
			wg.Wait()

			// Exactly one callback should be invoked
			total := atomic.LoadInt32(&ackCount) + atomic.LoadInt32(&nackCount)
			if total != 1 {
				t.Errorf("total callbacks = %d, want 1", total)
			}
		}
	})

	t.Run("concurrent context access during settlement", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		var wg sync.WaitGroup
		const readers = 10

		// Start readers
		for i := 0; i < readers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					ctx := msg.Context()
					_ = ctx.Err()
					_ = ctx.Done()
				}
			}()
		}

		// Settle the message
		time.Sleep(time.Millisecond)
		msg.Ack()

		wg.Wait()
		// No panic = success
	})
}

// TestAckingCallbackSafety tests that callbacks are invoked outside the mutex.
func TestAckingCallbackSafety(t *testing.T) {
	t.Run("self-referencing callback does not deadlock", func(t *testing.T) {
		// This is the key test for the deadlock fix.
		// If callbacks were called inside the mutex, this would deadlock.
		done := make(chan struct{})

		var msg *Message
		acking := NewAcking(
			func() {
				// Try to access acking state from callback
				_ = msg.AckState()
				close(done)
			},
			func(error) {},
		)
		msg = New("data", nil, acking)

		msg.Ack()

		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(time.Second):
			t.Fatal("deadlock detected - callback blocked")
		}
	})

	t.Run("callback can call Ack on same acking", func(t *testing.T) {
		// Multiple ack calls should be safe even from callback
		done := make(chan struct{})
		acking := NewSharedAcking(func() { close(done) }, func(error) {}, 2)

		msg1 := New("data1", nil, acking)
		var msg2 *Message
		msg2 = New("data2", nil, NewAcking(
			func() {
				// Ack msg1 from msg2's callback
				msg1.Ack()
			},
			func(error) {},
		))

		// First, ack one message
		msg1.Ack()
		// Now ack msg2, whose callback will try to ack msg1 again (idempotent)
		msg2.Ack()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for ack")
		}
	})

	t.Run("nack callback can access message state", func(t *testing.T) {
		done := make(chan struct{})

		var msg *Message
		acking := NewAcking(
			func() {},
			func(err error) {
				// Access message from nack callback
				_ = msg.AckState()
				_ = msg.Context()
				close(done)
			},
		)
		msg = New("data", nil, acking)

		msg.Nack(errors.New("test"))

		select {
		case <-done:
			// Success
		case <-time.After(time.Second):
			t.Fatal("deadlock in nack callback")
		}
	})
}

// TestAckingEdgeCases tests unusual but valid scenarios.
func TestAckingEdgeCases(t *testing.T) {
	t.Run("NewAcking with nil ack returns nil", func(t *testing.T) {
		if got := NewAcking(nil, func(error) {}); got != nil {
			t.Errorf("NewAcking(nil, fn) = %v, want nil", got)
		}
	})

	t.Run("NewAcking with nil nack returns nil", func(t *testing.T) {
		if got := NewAcking(func() {}, nil); got != nil {
			t.Errorf("NewAcking(fn, nil) = %v, want nil", got)
		}
	})

	t.Run("NewSharedAcking with zero count returns nil", func(t *testing.T) {
		if got := NewSharedAcking(func() {}, func(error) {}, 0); got != nil {
			t.Errorf("NewSharedAcking with count=0 = %v, want nil", got)
		}
	})

	t.Run("NewSharedAcking with negative count returns nil", func(t *testing.T) {
		if got := NewSharedAcking(func() {}, func(error) {}, -1); got != nil {
			t.Errorf("NewSharedAcking with count=-1 = %v, want nil", got)
		}
	})

	t.Run("Acking.State on nil returns AckPending", func(t *testing.T) {
		var a *Acking
		if got := a.State(); got != AckPending {
			t.Errorf("nil.State() = %v, want AckPending", got)
		}
	})

	t.Run("Acking.Context on nil returns Background", func(t *testing.T) {
		var a *Acking
		ctx := a.Context()
		if ctx.Done() != nil {
			t.Error("nil.Context().Done() should return nil")
		}
		if ctx.Err() != nil {
			t.Errorf("nil.Context().Err() = %v, want nil", ctx.Err())
		}
	})

	t.Run("message with nil acking - Ack returns false", func(t *testing.T) {
		msg := New("data", nil, nil)
		if msg.Ack() {
			t.Error("Ack() on nil acking should return false")
		}
	})

	t.Run("message with nil acking - Nack returns false", func(t *testing.T) {
		msg := New("data", nil, nil)
		if msg.Nack(errors.New("fail")) {
			t.Error("Nack() on nil acking should return false")
		}
	})

	t.Run("message with nil acking - AckState returns AckPending", func(t *testing.T) {
		msg := New("data", nil, nil)
		if got := msg.AckState(); got != AckPending {
			t.Errorf("AckState() on nil acking = %v, want AckPending", got)
		}
	})

	t.Run("message with nil acking - Done returns nil", func(t *testing.T) {
		msg := New("data", nil, nil)
		if got := msg.Done(); got != nil {
			t.Errorf("Done() on nil acking = %v, want nil", got)
		}
	})

	t.Run("message with nil acking - Context returns Background", func(t *testing.T) {
		msg := New("data", nil, nil)
		ctx := msg.Context()
		if ctx != context.Background() {
			t.Errorf("Context() on nil acking != context.Background()")
		}
	})

	t.Run("TypedMessage works with custom types", func(t *testing.T) {
		type CustomData struct {
			ID   int
			Name string
		}

		var acked bool
		acking := NewAcking(func() { acked = true }, func(error) {})
		msg := NewTyped(CustomData{ID: 1, Name: "test"}, nil, acking)

		if msg.Data.ID != 1 {
			t.Errorf("Data.ID = %d, want 1", msg.Data.ID)
		}

		msg.Ack()
		if !acked {
			t.Error("expected ack callback")
		}
	})
}

// TestMessageContextHelpers tests MessageFromContext and RawMessageFromContext.
func TestMessageContextHelpers(t *testing.T) {
	t.Run("MessageFromContext returns message", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("test data", Attributes{"type": "test"}, acking)
		ctx := msg.Context()

		got := MessageFromContext(ctx)
		if got != msg {
			t.Errorf("MessageFromContext() = %v, want %v", got, msg)
		}
	})

	t.Run("MessageFromContext returns nil for background context", func(t *testing.T) {
		ctx := context.Background()
		got := MessageFromContext(ctx)
		if got != nil {
			t.Errorf("MessageFromContext(Background) = %v, want nil", got)
		}
	})

	t.Run("MessageFromContext returns nil for nil acking message", func(t *testing.T) {
		msg := New("data", nil, nil)
		ctx := msg.Context() // Returns Background
		got := MessageFromContext(ctx)
		if got != nil {
			t.Errorf("MessageFromContext() = %v, want nil", got)
		}
	})

	t.Run("RawMessageFromContext returns raw message", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := NewRaw([]byte("raw data"), Attributes{"type": "raw.test"}, acking)
		ctx := msg.Context()

		got := RawMessageFromContext(ctx)
		if got != msg {
			t.Errorf("RawMessageFromContext() = %v, want %v", got, msg)
		}
	})

	t.Run("RawMessageFromContext returns nil for Message", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("not raw", nil, acking)
		ctx := msg.Context()

		got := RawMessageFromContext(ctx)
		if got != nil {
			t.Errorf("RawMessageFromContext() for Message = %v, want nil", got)
		}
	})

	t.Run("AttributesFromContext returns attributes", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		attrs := Attributes{"type": "test", "source": "/test"}
		msg := New("data", attrs, acking)
		ctx := msg.Context()

		got := AttributesFromContext(ctx)
		if got["type"] != "test" {
			t.Errorf("AttributesFromContext().type = %v, want test", got["type"])
		}
		if got["source"] != "/test" {
			t.Errorf("AttributesFromContext().source = %v, want /test", got["source"])
		}
	})

	t.Run("context values preserved after settlement", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", Attributes{"key": "value"}, acking)
		ctx := msg.Context()

		// Settle the message
		msg.Ack()

		// Context should still have message
		got := MessageFromContext(ctx)
		if got != msg {
			t.Errorf("MessageFromContext() after ack = %v, want %v", got, msg)
		}

		// Attributes should still be accessible
		attrs := AttributesFromContext(ctx)
		if attrs["key"] != "value" {
			t.Errorf("AttributesFromContext() after ack = %v, want value", attrs["key"])
		}
	})
}

// TestAckingStateTransitions tests all valid state transitions.
func TestAckingStateTransitions(t *testing.T) {
	tests := []struct {
		name       string
		actions    []string // "ack" or "nack"
		wantState  AckState
		wantReturns []bool
	}{
		{
			name:       "pending -> done (single ack)",
			actions:    []string{"ack"},
			wantState:  AckDone,
			wantReturns: []bool{true},
		},
		{
			name:       "pending -> nacked (single nack)",
			actions:    []string{"nack"},
			wantState:  AckNacked,
			wantReturns: []bool{true},
		},
		{
			name:       "done -> done (idempotent ack)",
			actions:    []string{"ack", "ack"},
			wantState:  AckDone,
			wantReturns: []bool{true, true},
		},
		{
			name:       "nacked -> nacked (idempotent nack)",
			actions:    []string{"nack", "nack"},
			wantState:  AckNacked,
			wantReturns: []bool{true, true},
		},
		{
			name:       "done -> done (ack after ack, blocked)",
			actions:    []string{"ack", "nack"},
			wantState:  AckDone,
			wantReturns: []bool{true, false}, // nack returns false
		},
		{
			name:       "nacked -> nacked (nack after nack, blocked)",
			actions:    []string{"nack", "ack"},
			wantState:  AckNacked,
			wantReturns: []bool{true, false}, // ack returns false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := New("data", nil, NewAcking(func() {}, func(error) {}))

			for i, action := range tt.actions {
				var got bool
				switch action {
				case "ack":
					got = msg.Ack()
				case "nack":
					got = msg.Nack(errors.New("fail"))
				}

				if got != tt.wantReturns[i] {
					t.Errorf("action[%d] %s returned %v, want %v", i, action, got, tt.wantReturns[i])
				}
			}

			if got := msg.AckState(); got != tt.wantState {
				t.Errorf("final state = %v, want %v", got, tt.wantState)
			}
		})
	}
}

// TestSharedAckingWithForwardPattern tests the pattern used by ForwardAck middleware.
func TestSharedAckingWithForwardPattern(t *testing.T) {
	t.Run("input acked when all outputs acked", func(t *testing.T) {
		var inputAcked bool
		inputAcking := NewAcking(
			func() { inputAcked = true },
			func(error) {},
		)
		input := New("input", nil, inputAcking)

		// Create outputs with shared acking that forwards to input
		outputAcking := NewSharedAcking(
			func() { input.Ack() },
			func(err error) { input.Nack(err) },
			3,
		)

		outputs := []*Message{
			New("out1", nil, outputAcking),
			New("out2", nil, outputAcking),
			New("out3", nil, outputAcking),
		}

		// Ack all outputs
		for _, out := range outputs {
			out.Ack()
		}

		if !inputAcked {
			t.Error("input should be acked after all outputs acked")
		}
	})

	t.Run("input nacked when any output nacked", func(t *testing.T) {
		var inputNacked bool
		inputAcking := NewAcking(
			func() {},
			func(error) { inputNacked = true },
		)
		input := New("input", nil, inputAcking)

		outputAcking := NewSharedAcking(
			func() { input.Ack() },
			func(err error) { input.Nack(err) },
			3,
		)

		outputs := []*Message{
			New("out1", nil, outputAcking),
			New("out2", nil, outputAcking),
			New("out3", nil, outputAcking),
		}

		// Ack first, nack second
		outputs[0].Ack()
		outputs[1].Nack(errors.New("fail"))

		if !inputNacked {
			t.Error("input should be nacked when any output nacks")
		}

		// Third output ack should be no-op
		result := outputs[2].Ack()
		if result {
			t.Error("ack after nack should return false")
		}
	})

	t.Run("sibling context cancelled on nack", func(t *testing.T) {
		inputAcking := NewAcking(func() {}, func(error) {})
		input := New("input", nil, inputAcking)

		outputAcking := NewSharedAcking(
			func() { input.Ack() },
			func(err error) { input.Nack(err) },
			2,
		)

		out1 := New("out1", nil, outputAcking)
		out2 := New("out2", nil, outputAcking)

		ctx1 := out1.Context()
		ctx2 := out2.Context()

		// Contexts should not be cancelled yet
		if ctx1.Err() != nil || ctx2.Err() != nil {
			t.Error("contexts should not be cancelled before nack")
		}

		// Nack one output
		out1.Nack(errors.New("fail"))

		// Both contexts should now be cancelled (same acking)
		if ctx1.Err() == nil {
			t.Error("ctx1 should be cancelled after nack")
		}
		if ctx2.Err() == nil {
			t.Error("ctx2 should be cancelled after nack (shared acking)")
		}
	})
}

// TestCopyPreservesAcking tests that Copy preserves acking reference.
func TestCopyPreservesAcking(t *testing.T) {
	t.Run("copied message shares acking", func(t *testing.T) {
		var acked bool
		acking := NewAcking(func() { acked = true }, func(error) {})
		original := New("original", Attributes{"key": "value"}, acking)

		copied := Copy(original, "copied data")

		// Verify data changed
		if copied.Data != "copied data" {
			t.Errorf("copied data = %v, want 'copied data'", copied.Data)
		}

		// Verify attributes cloned
		if copied.Attributes["key"] != "value" {
			t.Errorf("copied attributes = %v, want key=value", copied.Attributes)
		}

		// Verify acking shared - acking the copy should ack the original's callback
		copied.Ack()
		if !acked {
			t.Error("acking copied message should invoke original callback")
		}
	})

	t.Run("shared acking across copies", func(t *testing.T) {
		var ackCount int32
		acking := NewSharedAcking(
			func() { atomic.AddInt32(&ackCount, 1) },
			func(error) {},
			2,
		)
		msg1 := New("msg1", nil, acking)
		msg2 := Copy(msg1, "msg2")

		msg1.Ack()
		if atomic.LoadInt32(&ackCount) != 0 {
			t.Error("ack should not be called until both messages ack")
		}

		msg2.Ack()
		if atomic.LoadInt32(&ackCount) != 1 {
			t.Errorf("ack count = %d, want 1", atomic.LoadInt32(&ackCount))
		}
	})
}
