package middleware_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/middleware"
)

func TestDeadline(t *testing.T) {
	t.Run("passes through when no expirytime", func(t *testing.T) {
		called := false
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			called = true
			return nil, nil
		}

		mw := middleware.Deadline()
		wrapped := mw(handler)

		msg := message.New("data", message.Attributes{
			"type": "test.event",
		}, nil)

		_, err := wrapped(context.Background(), msg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !called {
			t.Error("expected handler to be called")
		}
	})

	t.Run("returns ErrMessageExpired for past expiry", func(t *testing.T) {
		called := false
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			called = true
			return nil, nil
		}

		mw := middleware.Deadline()
		wrapped := mw(handler)

		pastTime := time.Now().Add(-1 * time.Hour)
		msg := message.New("data", message.Attributes{
			"type":                  "test.event",
			message.AttrExpiryTime: pastTime,
		}, nil)

		_, err := wrapped(context.Background(), msg)
		if !errors.Is(err, middleware.ErrMessageExpired) {
			t.Errorf("expected ErrMessageExpired, got %v", err)
		}
		if called {
			t.Error("expected handler to not be called")
		}
	})

	t.Run("sets context deadline for future expiry", func(t *testing.T) {
		var receivedDeadline time.Time
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			d, ok := ctx.Deadline()
			if ok {
				receivedDeadline = d
			}
			return nil, nil
		}

		mw := middleware.Deadline()
		wrapped := mw(handler)

		futureTime := time.Now().Add(1 * time.Hour)
		msg := message.New("data", message.Attributes{
			"type":                  "test.event",
			message.AttrExpiryTime: futureTime,
		}, nil)

		_, err := wrapped(context.Background(), msg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Allow 1 second tolerance for comparison
		if receivedDeadline.Sub(futureTime).Abs() > time.Second {
			t.Errorf("expected deadline around %v, got %v", futureTime, receivedDeadline)
		}
	})

	t.Run("handles RFC3339 string expirytime", func(t *testing.T) {
		var receivedDeadline time.Time
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			d, ok := ctx.Deadline()
			if ok {
				receivedDeadline = d
			}
			return nil, nil
		}

		mw := middleware.Deadline()
		wrapped := mw(handler)

		futureTime := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
		msg := message.New("data", message.Attributes{
			"type":                  "test.event",
			message.AttrExpiryTime: futureTime.Format(time.RFC3339),
		}, nil)

		_, err := wrapped(context.Background(), msg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if receivedDeadline.Sub(futureTime).Abs() > time.Second {
			t.Errorf("expected deadline around %v, got %v", futureTime, receivedDeadline)
		}
	})

	t.Run("wraps DeadlineExceeded with ErrMessageExpired", func(t *testing.T) {
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			// Simulate slow processing that exceeds deadline
			<-ctx.Done()
			return nil, ctx.Err()
		}

		mw := middleware.Deadline()
		wrapped := mw(handler)

		// Very short expiry to trigger deadline
		shortExpiry := time.Now().Add(10 * time.Millisecond)
		msg := message.New("data", message.Attributes{
			"type":                "test.event",
			message.AttrExpiryTime: shortExpiry,
		}, nil)

		_, err := wrapped(context.Background(), msg)

		// Should be wrapped with ErrMessageExpired
		if !errors.Is(err, middleware.ErrMessageExpired) {
			t.Errorf("expected ErrMessageExpired, got %v", err)
		}
		// Should also still be DeadlineExceeded
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
	})

	t.Run("preserves earlier parent context deadline", func(t *testing.T) {
		var receivedDeadline time.Time
		handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
			d, ok := ctx.Deadline()
			if ok {
				receivedDeadline = d
			}
			return nil, nil
		}

		mw := middleware.Deadline()
		wrapped := mw(handler)

		// Parent context with earlier deadline
		parentDeadline := time.Now().Add(30 * time.Minute)
		parentCtx, cancel := context.WithDeadline(context.Background(), parentDeadline)
		defer cancel()

		// Message with later expiry time
		laterExpiry := time.Now().Add(1 * time.Hour)
		msg := message.New("data", message.Attributes{
			"type":                  "test.event",
			message.AttrExpiryTime: laterExpiry,
		}, nil)

		_, err := wrapped(parentCtx, msg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Should preserve the earlier parent deadline
		if receivedDeadline.Sub(parentDeadline).Abs() > time.Second {
			t.Errorf("expected earlier parent deadline %v, got %v", parentDeadline, receivedDeadline)
		}
	})
}
