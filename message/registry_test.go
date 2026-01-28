package message

import (
	"context"
	"testing"
)

type TestOrder struct {
	ID string
}

func TestFactoryMap(t *testing.T) {
	t.Run("NewInput returns instance for registered type", func(t *testing.T) {
		registry := FactoryMap{
			"test.order": func() any { return &TestOrder{} },
		}

		instance := registry.NewInput("test.order")
		if instance == nil {
			t.Fatal("NewInput() returned nil for registered type")
		}

		order, ok := instance.(*TestOrder)
		if !ok {
			t.Fatalf("NewInput() returned %T, want *TestOrder", instance)
		}
		if order.ID != "" {
			t.Errorf("order.ID = %q, want empty", order.ID)
		}
	})

	t.Run("NewInput returns nil for unknown type", func(t *testing.T) {
		registry := FactoryMap{}

		instance := registry.NewInput("unknown.type")
		if instance != nil {
			t.Errorf("NewInput() = %v, want nil", instance)
		}
	})
}

func TestRouter_InputRegistry(t *testing.T) {
	t.Run("NewInput returns instance for registered handler", func(t *testing.T) {
		router := NewRouter(PipeConfig{})
		handler := NewHandler[TestOrder](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			KebabNaming,
		)
		_ = router.AddHandler("", nil, handler)

		instance := router.NewInput("test.order")
		if instance == nil {
			t.Fatal("NewInput() returned nil for registered handler")
		}

		order, ok := instance.(*TestOrder)
		if !ok {
			t.Fatalf("NewInput() returned %T, want *TestOrder", instance)
		}
		if order.ID != "" {
			t.Errorf("order.ID = %q, want empty", order.ID)
		}
	})

	t.Run("NewInput returns nil for unknown type", func(t *testing.T) {
		router := NewRouter(PipeConfig{})

		instance := router.NewInput("unknown.type")
		if instance != nil {
			t.Errorf("NewInput() = %v, want nil", instance)
		}
	})
}
