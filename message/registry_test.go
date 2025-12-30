package message

import (
	"context"
	"testing"
)

type TestOrder struct {
	ID string
}

func TestFactoryMap(t *testing.T) {
	t.Run("NewInstance returns instance for registered type", func(t *testing.T) {
		registry := FactoryMap{
			"test.order": func() any { return &TestOrder{} },
		}

		instance := registry.NewInstance("test.order")
		if instance == nil {
			t.Fatal("NewInstance() returned nil for registered type")
		}

		order, ok := instance.(*TestOrder)
		if !ok {
			t.Fatalf("NewInstance() returned %T, want *TestOrder", instance)
		}
		if order.ID != "" {
			t.Errorf("order.ID = %q, want empty", order.ID)
		}
	})

	t.Run("NewInstance returns nil for unknown type", func(t *testing.T) {
		registry := FactoryMap{}

		instance := registry.NewInstance("unknown.type")
		if instance != nil {
			t.Errorf("NewInstance() = %v, want nil", instance)
		}
	})
}

func TestRouter_TypeRegistry(t *testing.T) {
	t.Run("NewInstance returns instance for registered handler", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		handler := NewHandler[TestOrder](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			KebabNaming,
		)
		router.AddHandler(handler, HandlerConfig{})

		instance := router.NewInstance("test.order")
		if instance == nil {
			t.Fatal("NewInstance() returned nil for registered handler")
		}

		order, ok := instance.(*TestOrder)
		if !ok {
			t.Fatalf("NewInstance() returned %T, want *TestOrder", instance)
		}
		if order.ID != "" {
			t.Errorf("order.ID = %q, want empty", order.ID)
		}
	})

	t.Run("NewInstance returns nil for unknown type", func(t *testing.T) {
		router := NewRouter(RouterConfig{})

		instance := router.NewInstance("unknown.type")
		if instance != nil {
			t.Errorf("NewInstance() = %v, want nil", instance)
		}
	})
}
