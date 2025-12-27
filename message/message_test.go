package message

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("creates message with data and attributes", func(t *testing.T) {
		data := "test data"
		attrs := Attributes{"key": "value"}
		msg := New(data, attrs)

		if msg.Data != data {
			t.Errorf("expected data %q, got %q", data, msg.Data)
		}
		if msg.Attributes["key"] != "value" {
			t.Errorf("expected attribute key=value, got %v", msg.Attributes["key"])
		}
	})

	t.Run("creates empty attributes when nil", func(t *testing.T) {
		msg := New("data", nil)
		if msg.Attributes == nil {
			t.Error("expected non-nil attributes map")
		}
	})
}

func TestNewWithAcking(t *testing.T) {
	t.Run("ack callback is invoked", func(t *testing.T) {
		acked := false
		msg := NewWithAcking("data", nil, func() { acked = true }, func(error) {})

		msg.Ack()
		if !acked {
			t.Error("expected ack callback to be invoked")
		}
	})

	t.Run("nack callback is invoked with error", func(t *testing.T) {
		var nackErr error
		msg := NewWithAcking("data", nil, func() {}, func(err error) { nackErr = err })

		testErr := ErrNoHandler
		msg.Nack(testErr)
		if nackErr != testErr {
			t.Errorf("expected nack error %v, got %v", testErr, nackErr)
		}
	})
}

func TestAcking(t *testing.T) {
	t.Run("ack requires expected count", func(t *testing.T) {
		acked := false
		acking := NewAcking(func() { acked = true }, func(error) {}, 3)

		msg1 := NewWithSharedAcking("data1", nil, acking)
		msg2 := NewWithSharedAcking("data2", nil, acking)
		msg3 := NewWithSharedAcking("data3", nil, acking)

		msg1.Ack()
		if acked {
			t.Error("acked too early after 1 ack")
		}

		msg2.Ack()
		if acked {
			t.Error("acked too early after 2 acks")
		}

		msg3.Ack()
		if !acked {
			t.Error("expected ack after 3 acks")
		}
	})

	t.Run("nack blocks further acks", func(t *testing.T) {
		acked := false
		nacked := false
		acking := NewAcking(func() { acked = true }, func(error) { nacked = true }, 2)

		msg1 := NewWithSharedAcking("data1", nil, acking)
		msg2 := NewWithSharedAcking("data2", nil, acking)

		msg1.Nack(ErrNoHandler)
		if !nacked {
			t.Error("expected nack callback")
		}

		result := msg2.Ack()
		if result {
			t.Error("expected Ack to return false after Nack")
		}
		if acked {
			t.Error("ack callback should not be invoked after nack")
		}
	})

	t.Run("ack is idempotent", func(t *testing.T) {
		count := 0
		msg := NewWithAcking("data", nil, func() { count++ }, func(error) {})

		msg.Ack()
		msg.Ack()
		msg.Ack()

		if count != 1 {
			t.Errorf("expected ack callback once, got %d", count)
		}
	})

	t.Run("nack is idempotent", func(t *testing.T) {
		count := 0
		msg := NewWithAcking("data", nil, func() {}, func(error) { count++ })

		msg.Nack(ErrNoHandler)
		msg.Nack(ErrNoHandler)

		if count != 1 {
			t.Errorf("expected nack callback once, got %d", count)
		}
	})

	t.Run("thread safety", func(t *testing.T) {
		acked := false
		acking := NewAcking(func() { acked = true }, func(error) {}, 100)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				msg := NewWithSharedAcking("data", nil, acking)
				msg.Ack()
			}()
		}
		wg.Wait()

		if !acked {
			t.Error("expected ack after 100 concurrent acks")
		}
	})

	t.Run("nil acking returns false", func(t *testing.T) {
		msg := New[string]("data", nil)
		if msg.Ack() {
			t.Error("expected Ack to return false with nil acking")
		}
		if msg.Nack(ErrNoHandler) {
			t.Error("expected Nack to return false with nil acking")
		}
	})

	t.Run("NewAcking returns nil for invalid params", func(t *testing.T) {
		if NewAcking(nil, func(error) {}, 1) != nil {
			t.Error("expected nil for nil ack")
		}
		if NewAcking(func() {}, nil, 1) != nil {
			t.Error("expected nil for nil nack")
		}
		if NewAcking(func() {}, func(error) {}, 0) != nil {
			t.Error("expected nil for zero count")
		}
		if NewAcking(func() {}, func(error) {}, -1) != nil {
			t.Error("expected nil for negative count")
		}
	})
}

func TestCopy(t *testing.T) {
	t.Run("preserves attributes and acking", func(t *testing.T) {
		acked := false
		original := NewWithAcking("original", Attributes{"key": "value"}, func() { acked = true }, func(error) {})

		copied := Copy(original, "copied")

		if copied.Data != "copied" {
			t.Errorf("expected copied data, got %v", copied.Data)
		}
		if copied.Attributes["key"] != "value" {
			t.Error("expected attributes to be preserved")
		}

		copied.Ack()
		if !acked {
			t.Error("expected shared acking to work")
		}
	})
}
