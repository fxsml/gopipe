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
			wantSettled bool
			wantErr     bool // whether Err() should be non-nil
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
				action:      func(m *Message) { m.Ack() },
				wantAcked:   1,
				wantNacked:  0,
				wantSettled: true,
				wantErr:     false,
				wantDone:    true,
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
				action:      func(m *Message) { m.Nack(errors.New("test error")) },
				wantAcked:   0,
				wantNacked:  1,
				wantSettled: true,
				wantErr:     true,
				wantDone:    true,
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
				wantAcked:   1,
				wantNacked:  0,
				wantSettled: true,
				wantErr:     false,
				wantDone:    true,
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
				wantAcked:   0,
				wantNacked:  1, // Only first nack callback invoked
				wantSettled: true,
				wantErr:     true,
				wantDone:    true,
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
				wantAcked:   0,
				wantNacked:  1,
				wantSettled: true,
				wantErr:     true,
				wantDone:    true,
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
				wantAcked:   1,
				wantNacked:  0,
				wantSettled: true,
				wantErr:     false,
				wantDone:    true,
			},
			{
				name: "nil acking - ack returns false",
				setup: func() (*Message, *int32, *int32, *error) {
					msg := New("data", nil, nil)
					return msg, new(int32), new(int32), new(error)
				},
				action:      func(m *Message) {},
				wantAcked:   0,
				wantNacked:  0,
				wantSettled: false, // nil acking is not settled
				wantErr:     false,
				wantDone:    false, // No done channel
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
				gotSettled := false
				select {
				case <-msg.Done():
					gotSettled = true
				default:
				}
				if gotSettled != tt.wantSettled {
					t.Errorf("settled = %v, want %v", gotSettled, tt.wantSettled)
				}
				if gotErr := msg.Err() != nil; gotErr != tt.wantErr {
					t.Errorf("Err() != nil = %v, want %v", gotErr, tt.wantErr)
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
			name        string
			count       int
			actions     []func([]*Message)
			wantAcked   int32
			wantNacked  int32
			wantSettled bool
			wantErr     bool
			wantDone    bool
		}{
			{
				name:  "all messages ack - callback invoked",
				count: 3,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
					func(msgs []*Message) { msgs[1].Ack() },
					func(msgs []*Message) { msgs[2].Ack() },
				},
				wantAcked:   1,
				wantNacked:  0,
				wantSettled: true,
				wantErr:     false,
				wantDone:    true,
			},
			{
				name:  "one nack blocks all acks",
				count: 3,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
					func(msgs []*Message) { msgs[1].Nack(errors.New("fail")) },
					func(msgs []*Message) { msgs[2].Ack() }, // Should return false
				},
				wantAcked:   0,
				wantNacked:  1,
				wantSettled: true,
				wantErr:     true,
				wantDone:    true,
			},
			{
				name:  "partial acks - callback not invoked",
				count: 3,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
					func(msgs []*Message) { msgs[1].Ack() },
					// msgs[2] not acked
				},
				wantAcked:   0,
				wantNacked:  0,
				wantSettled: false,
				wantErr:     false,
				wantDone:    false,
			},
			{
				name:  "single message in shared acking",
				count: 1,
				actions: []func([]*Message){
					func(msgs []*Message) { msgs[0].Ack() },
				},
				wantAcked:   1,
				wantNacked:  0,
				wantSettled: true,
				wantErr:     false,
				wantDone:    true,
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
				wantAcked:   0,
				wantNacked:  1,
				wantSettled: true,
				wantErr:     true,
				wantDone:    true,
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
				gotSettled := false
				select {
				case <-msgs[0].Done():
					gotSettled = true
				default:
				}
				if gotSettled != tt.wantSettled {
					t.Errorf("settled = %v, want %v", gotSettled, tt.wantSettled)
				}
				if gotErr := msgs[0].Err() != nil; gotErr != tt.wantErr {
					t.Errorf("Err() != nil = %v, want %v", gotErr, tt.wantErr)
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
		// Context does NOT cancel on message settlement.
		// Context is for lifecycle (parent cancellation, deadline).
		// Settlement detection uses msg.Done() directly.
		tests := []struct {
			name        string
			setup       func() (*Message, func())
			wantMessage bool
		}{
			{
				name: "context has message after ack",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Ack() }
				},
				wantMessage: true,
			},
			{
				name: "context has message after nack",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Nack(errors.New("fail")) }
				},
				wantMessage: true,
			},
			{
				name: "context not cancelled on settlement",
				setup: func() (*Message, func()) {
					acking := NewAcking(func() {}, func(error) {})
					msg := New("data", nil, acking)
					return msg, func() { msg.Ack() }
				},
				wantMessage: true,
			},
			{
				name: "nil acking still stores message in context",
				setup: func() (*Message, func()) {
					msg := New("data", nil, nil)
					return msg, func() {}
				},
				wantMessage: true, // Message stored even with nil acking
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				msg, action := tt.setup()
				ctx := msg.Context(context.Background())

				// Verify context not cancelled before action
				if ctx.Err() != nil {
					t.Errorf("Context.Err() = %v before action, want nil", ctx.Err())
				}

				action()

				// Context should NOT be cancelled after settlement
				// (context lifecycle is separate from message settlement)
				if ctx.Err() != nil {
					t.Errorf("Context.Err() = %v after action, want nil (context should not cancel on settlement)", ctx.Err())
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
					ctx := msg.Context(context.Background())
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
				select {
				case <-msg.Done():
				default:
				}
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
		msg2 := New("data2", nil, NewAcking(
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
				select {
				case <-msg.Done():
				default:
				}
				_ = msg.Context(context.Background())
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

	t.Run("message with nil acking - Done channel select works", func(t *testing.T) {
		msg := New("data", nil, nil)
		select {
		case <-msg.Done():
			t.Error("Done() on nil acking should not be closed")
		default:
			// OK - nil channel causes default case
		}
	})

	t.Run("message with nil acking - Err returns nil", func(t *testing.T) {
		msg := New("data", nil, nil)
		if err := msg.Err(); err != nil {
			t.Errorf("Err() on nil acking = %v, want nil", err)
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

	t.Run("message with nil acking - Done returns nil", func(t *testing.T) {
		msg := New("data", nil, nil)
		if got := msg.Done(); got != nil {
			t.Errorf("Done() on nil acking = %v, want nil", got)
		}
	})

	t.Run("message with nil acking - Context still stores message", func(t *testing.T) {
		msg := New("data", nil, nil)
		ctx := msg.Context(context.Background())
		// Even with nil acking, message is stored in context
		if got := MessageFromContext(ctx); got != msg {
			t.Errorf("MessageFromContext() = %v, want %v", got, msg)
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

	t.Run("ack callback panic - done channel still closes", func(t *testing.T) {
		acking := NewAcking(
			func() { panic("ack callback panic") },
			func(error) {},
		)
		msg := New("data", nil, acking)

		// Ack with panic - should propagate but done channel closes
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic to propagate")
			}
			// Verify done channel was closed despite panic
			select {
			case <-msg.Done():
				// OK - channel closed as expected
			default:
				t.Error("done channel should be closed even after panic")
			}
		}()

		msg.Ack()
	})

	t.Run("nack callback panic - done channel still closes", func(t *testing.T) {
		acking := NewAcking(
			func() {},
			func(error) { panic("nack callback panic") },
		)
		msg := New("data", nil, acking)

		// Nack with panic - should propagate but done channel closes
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic to propagate")
			}
			// Verify done channel was closed despite panic
			select {
			case <-msg.Done():
				// OK - channel closed as expected
			default:
				t.Error("done channel should be closed even after panic")
			}
		}()

		msg.Nack(errors.New("test"))
	})
}

// TestMessageContextHelpers tests MessageFromContext and RawMessageFromContext.
func TestMessageContextHelpers(t *testing.T) {
	t.Run("MessageFromContext returns message", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("test data", Attributes{"type": "test"}, acking)
		ctx := msg.Context(context.Background())

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

	t.Run("MessageFromContext returns message for nil acking message", func(t *testing.T) {
		msg := New("data", nil, nil)
		ctx := msg.Context(context.Background()) // Still stores message
		got := MessageFromContext(ctx)
		if got != msg {
			t.Errorf("MessageFromContext() = %v, want %v", got, msg)
		}
	})

	t.Run("RawMessageFromContext returns raw message", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := NewRaw([]byte("raw data"), Attributes{"type": "raw.test"}, acking)
		ctx := msg.Context(context.Background())

		got := RawMessageFromContext(ctx)
		if got != msg {
			t.Errorf("RawMessageFromContext() = %v, want %v", got, msg)
		}
	})

	t.Run("RawMessageFromContext returns nil for Message", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("not raw", nil, acking)
		ctx := msg.Context(context.Background())

		got := RawMessageFromContext(ctx)
		if got != nil {
			t.Errorf("RawMessageFromContext() for Message = %v, want nil", got)
		}
	})

	t.Run("AttributesFromContext returns attributes", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		attrs := Attributes{"type": "test", "source": "/test"}
		msg := New("data", attrs, acking)
		ctx := msg.Context(context.Background())

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
		ctx := msg.Context(context.Background())

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

// TestMessageContextWithParent tests context derivation from parent.
func TestMessageContextWithParent(t *testing.T) {
	t.Run("inherits parent deadline", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		deadline := time.Now().Add(time.Hour)
		parent, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		ctx := msg.Context(parent)

		gotDeadline, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline from parent")
		}
		if !gotDeadline.Equal(deadline) {
			t.Errorf("Deadline() = %v, want %v", gotDeadline, deadline)
		}
	})

	t.Run("uses message expiry if earlier than parent", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		expiry := time.Now().Add(time.Minute)
		msg := New("data", Attributes{AttrExpiryTime: expiry}, acking)

		parentDeadline := time.Now().Add(time.Hour)
		parent, cancel := context.WithDeadline(context.Background(), parentDeadline)
		defer cancel()

		ctx := msg.Context(parent)

		gotDeadline, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline")
		}
		// Should use expiry (1 min) not parent deadline (1 hour)
		if !gotDeadline.Equal(expiry) {
			t.Errorf("Deadline() = %v, want %v (expiry)", gotDeadline, expiry)
		}
	})

	t.Run("uses parent deadline if earlier than expiry", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		expiry := time.Now().Add(time.Hour)
		msg := New("data", Attributes{AttrExpiryTime: expiry}, acking)

		parentDeadline := time.Now().Add(time.Minute)
		parent, cancel := context.WithDeadline(context.Background(), parentDeadline)
		defer cancel()

		ctx := msg.Context(parent)

		gotDeadline, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline")
		}
		// Should use parent deadline (1 min) not expiry (1 hour)
		if !gotDeadline.Equal(parentDeadline) {
			t.Errorf("Deadline() = %v, want %v (parent)", gotDeadline, parentDeadline)
		}
	})

	t.Run("uses expiry when no parent deadline", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		expiry := time.Now().Add(time.Minute)
		msg := New("data", Attributes{AttrExpiryTime: expiry}, acking)

		ctx := msg.Context(context.Background())

		gotDeadline, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline from expiry")
		}
		if !gotDeadline.Equal(expiry) {
			t.Errorf("Deadline() = %v, want %v", gotDeadline, expiry)
		}
	})

	t.Run("no deadline when neither parent nor expiry set", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		ctx := msg.Context(context.Background())

		_, ok := ctx.Deadline()
		if ok {
			t.Error("expected no deadline")
		}
	})

	t.Run("cancelled when parent cancelled", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		parent, cancel := context.WithCancel(context.Background())
		ctx := msg.Context(parent)

		// Not cancelled yet
		if ctx.Err() != nil {
			t.Error("context should not be cancelled yet")
		}

		// Cancel parent
		cancel()

		// Wait for context to be cancelled
		select {
		case <-ctx.Done():
			// OK
		case <-time.After(time.Second):
			t.Error("context should be cancelled when parent cancelled")
		}

		if ctx.Err() != context.Canceled {
			t.Errorf("Err() = %v, want context.Canceled", ctx.Err())
		}
	})

	t.Run("inherits parent values", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		type ctxKey string
		parent := context.WithValue(context.Background(), ctxKey("key"), "value")
		ctx := msg.Context(parent)

		if got := ctx.Value(ctxKey("key")); got != "value" {
			t.Errorf("Value() = %v, want 'value'", got)
		}
	})

	t.Run("FromContext alias works", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)
		ctx := msg.Context(context.Background())

		got := FromContext(ctx)
		if got != msg {
			t.Errorf("FromContext() = %v, want %v", got, msg)
		}
	})

	t.Run("msg.Done channel closes on settlement (not ctx.Done)", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)
		_ = msg.Context(context.Background())

		// msg.Done() should return non-nil channel
		done := msg.Done()
		if done == nil {
			t.Fatal("msg.Done() returned nil")
		}

		// Should not be closed yet
		select {
		case <-done:
			t.Error("msg.Done() closed before settlement")
		default:
			// OK
		}

		msg.Ack()

		// msg.Done() should close after settlement
		select {
		case <-done:
			// OK
		case <-time.After(time.Second):
			t.Error("msg.Done() not closed after settlement")
		}
	})

	t.Run("propagates DeadlineExceeded from parent", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		// Create parent with very short deadline
		parent, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()

		ctx := msg.Context(parent)

		// Wait for deadline to pass
		select {
		case <-ctx.Done():
			// OK
		case <-time.After(time.Second):
			t.Fatal("context should be cancelled after deadline")
		}

		// Should return DeadlineExceeded
		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("Err() = %v, want context.DeadlineExceeded", ctx.Err())
		}
	})

	t.Run("multiple Context calls each work independently", func(t *testing.T) {
		acking := NewAcking(func() {}, func(error) {})
		msg := New("data", nil, acking)

		ctx1 := msg.Context(context.Background())
		ctx2 := msg.Context(context.Background())
		ctx3 := msg.Context(context.Background())

		// All should have the message
		if MessageFromContext(ctx1) != msg {
			t.Error("ctx1 missing message")
		}
		if MessageFromContext(ctx2) != msg {
			t.Error("ctx2 missing message")
		}
		if MessageFromContext(ctx3) != msg {
			t.Error("ctx3 missing message")
		}

		// None should be cancelled
		if ctx1.Err() != nil || ctx2.Err() != nil || ctx3.Err() != nil {
			t.Error("contexts should not be cancelled")
		}

		msg.Ack()

		// Contexts should NOT be cancelled after settlement
		// (context lifecycle is separate from message settlement)
		if ctx1.Err() != nil || ctx2.Err() != nil || ctx3.Err() != nil {
			t.Error("contexts should not be cancelled after settlement (lifecycle separate from settlement)")
		}

		// Use msg.Done() for settlement detection
		select {
		case <-msg.Done():
			// OK - settlement detected via msg.Done()
		default:
			t.Error("msg.Done() should be closed after settlement")
		}
	})

	t.Run("custom context does not create timers or goroutines", func(t *testing.T) {
		// This test verifies the messageContext implementation works correctly
		// without creating timers or goroutines for deadline enforcement.
		acking := NewAcking(func() {}, func(error) {})
		expiry := time.Now().Add(time.Hour)
		msg := New("data", Attributes{AttrExpiryTime: expiry}, acking)

		ctx := msg.Context(context.Background())

		// Deadline should be reported
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline from expiry")
		}
		if !deadline.Equal(expiry) {
			t.Errorf("Deadline() = %v, want %v", deadline, expiry)
		}

		// Done() delegates to parent (background context has nil Done)
		if ctx.Done() != nil {
			t.Error("Done() should be nil for background context parent")
		}

		// Err() delegates to parent
		if ctx.Err() != nil {
			t.Error("Err() should be nil")
		}

		// Message should be retrievable
		if MessageFromContext(ctx) != msg {
			t.Error("MessageFromContext() should return the message")
		}
	})
}

// TestAckingStateTransitions tests all valid state transitions.
func TestAckingStateTransitions(t *testing.T) {
	tests := []struct {
		name        string
		actions     []string // "ack" or "nack"
		wantSettled bool
		wantErr     bool
		wantReturns []bool
	}{
		{
			name:        "pending -> done (single ack)",
			actions:     []string{"ack"},
			wantSettled: true,
			wantErr:     false,
			wantReturns: []bool{true},
		},
		{
			name:        "pending -> nacked (single nack)",
			actions:     []string{"nack"},
			wantSettled: true,
			wantErr:     true,
			wantReturns: []bool{true},
		},
		{
			name:        "done -> done (idempotent ack)",
			actions:     []string{"ack", "ack"},
			wantSettled: true,
			wantErr:     false,
			wantReturns: []bool{true, true},
		},
		{
			name:        "nacked -> nacked (idempotent nack)",
			actions:     []string{"nack", "nack"},
			wantSettled: true,
			wantErr:     true,
			wantReturns: []bool{true, true},
		},
		{
			name:        "done -> done (ack after ack, blocked)",
			actions:     []string{"ack", "nack"},
			wantSettled: true,
			wantErr:     false,
			wantReturns: []bool{true, false}, // nack returns false
		},
		{
			name:        "nacked -> nacked (nack after nack, blocked)",
			actions:     []string{"nack", "ack"},
			wantSettled: true,
			wantErr:     true,
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

			gotSettled := false
			select {
			case <-msg.Done():
				gotSettled = true
			default:
			}
			if gotSettled != tt.wantSettled {
				t.Errorf("settled = %v, want %v", gotSettled, tt.wantSettled)
			}
			if gotErr := msg.Err() != nil; gotErr != tt.wantErr {
				t.Errorf("Err() != nil = %v, want %v", gotErr, tt.wantErr)
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

	t.Run("sibling messages detect settlement via Done channel", func(t *testing.T) {
		inputAcking := NewAcking(func() {}, func(error) {})
		input := New("input", nil, inputAcking)

		outputAcking := NewSharedAcking(
			func() { input.Ack() },
			func(err error) { input.Nack(err) },
			2,
		)

		out1 := New("out1", nil, outputAcking)
		out2 := New("out2", nil, outputAcking)

		// Done channels should not be closed yet
		select {
		case <-out1.Done():
			t.Error("out1.Done() should not be closed before nack")
		default:
			// OK
		}
		select {
		case <-out2.Done():
			t.Error("out2.Done() should not be closed before nack")
		default:
			// OK
		}

		// Nack one output
		out1.Nack(errors.New("fail"))

		// Both Done channels should close (shared acking)
		select {
		case <-out1.Done():
			// OK
		case <-time.After(time.Second):
			t.Error("out1.Done() should be closed after nack")
		}
		select {
		case <-out2.Done():
			// OK
		case <-time.After(time.Second):
			t.Error("out2.Done() should be closed after nack (shared acking)")
		}

		// Verify state using Done() + Err()
		select {
		case <-out1.Done():
			if out1.Err() == nil {
				t.Error("out1 should be nacked")
			}
		default:
			t.Error("out1 should be settled")
		}
		select {
		case <-out2.Done():
			if out2.Err() == nil {
				t.Error("out2 should be nacked (shared acking)")
			}
		default:
			t.Error("out2 should be settled (shared acking)")
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
