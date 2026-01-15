package middleware_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/middleware"
)

func TestValidateRequired(t *testing.T) {
	passthrough := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
		return []*message.Message{msg}, nil
	}

	t.Run("passes with all required attributes", func(t *testing.T) {
		mw := middleware.ValidateRequired()
		wrapped := mw(passthrough)

		msg := message.New("data", message.Attributes{
			message.AttrID:          "123",
			message.AttrType:        "test.event",
			message.AttrSource:      "/test",
			message.AttrSpecVersion: "1.0",
		}, nil)

		outputs, err := wrapped(context.Background(), msg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(outputs) != 1 {
			t.Errorf("expected 1 output, got %d", len(outputs))
		}
	})

	t.Run("fails with single missing attribute", func(t *testing.T) {
		mw := middleware.ValidateRequired()
		wrapped := mw(passthrough)

		msg := message.New("data", message.Attributes{
			message.AttrType:        "test.event",
			message.AttrSource:      "/test",
			message.AttrSpecVersion: "1.0",
		}, nil)

		_, err := wrapped(context.Background(), msg)
		if !errors.Is(err, middleware.ErrMissingRequiredAttr) {
			t.Errorf("expected ErrMissingRequiredAttr, got %v", err)
		}
		if !strings.Contains(err.Error(), message.AttrID) {
			t.Errorf("expected error to mention %s, got %v", message.AttrID, err)
		}
	})

	t.Run("fails with multiple missing attributes", func(t *testing.T) {
		mw := middleware.ValidateRequired()
		wrapped := mw(passthrough)

		msg := message.New("data", message.Attributes{
			message.AttrSource: "/test",
		}, nil)

		_, err := wrapped(context.Background(), msg)
		if !errors.Is(err, middleware.ErrMissingRequiredAttr) {
			t.Errorf("expected ErrMissingRequiredAttr, got %v", err)
		}
		// Should mention all missing attributes
		if !strings.Contains(err.Error(), message.AttrID) {
			t.Errorf("expected error to mention %s, got %v", message.AttrID, err)
		}
		if !strings.Contains(err.Error(), message.AttrType) {
			t.Errorf("expected error to mention %s, got %v", message.AttrType, err)
		}
		if !strings.Contains(err.Error(), message.AttrSpecVersion) {
			t.Errorf("expected error to mention %s, got %v", message.AttrSpecVersion, err)
		}
	})

	t.Run("fails with no attributes", func(t *testing.T) {
		mw := middleware.ValidateRequired()
		wrapped := mw(passthrough)

		msg := message.New("data", nil, nil)

		_, err := wrapped(context.Background(), msg)
		if !errors.Is(err, middleware.ErrMissingRequiredAttr) {
			t.Errorf("expected ErrMissingRequiredAttr, got %v", err)
		}
	})
}
