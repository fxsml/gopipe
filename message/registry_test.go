package message

import (
	"context"
	"testing"
)

type TestOrder struct {
	ID string
}

func TestTypeOf(t *testing.T) {
	t.Run("creates TypeEntry with correct EventType", func(t *testing.T) {
		entry := TypeOf[TestOrder](KebabNaming)

		if entry.EventType() != "test.order" {
			t.Errorf("EventType() = %q, want %q", entry.EventType(), "test.order")
		}
	})

	t.Run("NewInstance returns pointer to type", func(t *testing.T) {
		entry := TypeOf[TestOrder](KebabNaming)

		instance := entry.NewInstance()
		if instance == nil {
			t.Fatal("NewInstance() returned nil")
		}

		order, ok := instance.(*TestOrder)
		if !ok {
			t.Fatalf("NewInstance() returned %T, want *TestOrder", instance)
		}

		// Should be zero value
		if order.ID != "" {
			t.Errorf("order.ID = %q, want empty", order.ID)
		}
	})
}

func TestMapRegistry(t *testing.T) {
	t.Run("Lookup returns registered entry", func(t *testing.T) {
		registry := MapRegistry{
			"test.order": func() any { return &TestOrder{} },
		}

		entry, ok := registry.Lookup("test.order")
		if !ok {
			t.Fatal("Lookup() returned false for registered type")
		}

		if entry.EventType() != "test.order" {
			t.Errorf("EventType() = %q, want %q", entry.EventType(), "test.order")
		}

		instance := entry.NewInstance()
		if _, ok := instance.(*TestOrder); !ok {
			t.Errorf("NewInstance() returned %T, want *TestOrder", instance)
		}
	})

	t.Run("Lookup returns false for unknown type", func(t *testing.T) {
		registry := MapRegistry{}

		_, ok := registry.Lookup("unknown.type")
		if ok {
			t.Error("Lookup() returned true for unknown type")
		}
	})

	t.Run("Register adds TypeEntry", func(t *testing.T) {
		registry := make(MapRegistry)
		entry := TypeOf[TestOrder](KebabNaming)

		err := registry.Register(entry)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		found, ok := registry.Lookup("test.order")
		if !ok {
			t.Fatal("Lookup() returned false after Register()")
		}

		if found.EventType() != entry.EventType() {
			t.Errorf("EventType() = %q, want %q", found.EventType(), entry.EventType())
		}
	})
}

func TestHandlerFunc(t *testing.T) {
	t.Run("Handle calls the function", func(t *testing.T) {
		called := false
		fn := HandlerFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
			called = true
			return []*Message{msg}, nil
		})

		msg := New[any]("test", nil)
		outputs, err := fn.Handle(context.Background(), msg)

		if err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
		if !called {
			t.Error("Handle() did not call the function")
		}
		if len(outputs) != 1 || outputs[0] != msg {
			t.Errorf("Handle() outputs = %v, want [%v]", outputs, msg)
		}
	})
}

func TestRouter_TypeRegistry(t *testing.T) {
	t.Run("Lookup returns registered handler as TypeEntry", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		handler := NewHandler[TestOrder](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			KebabNaming,
		)
		router.AddHandler(handler, HandlerConfig{})

		entry, ok := router.Lookup("test.order")
		if !ok {
			t.Fatal("Lookup() returned false for registered handler")
		}

		if entry.EventType() != "test.order" {
			t.Errorf("EventType() = %q, want %q", entry.EventType(), "test.order")
		}

		instance := entry.NewInstance()
		if _, ok := instance.(*TestOrder); !ok {
			t.Errorf("NewInstance() returned %T, want *TestOrder", instance)
		}
	})

	t.Run("Lookup returns false for unknown type", func(t *testing.T) {
		router := NewRouter(RouterConfig{})

		_, ok := router.Lookup("unknown.type")
		if ok {
			t.Error("Lookup() returned true for unknown type")
		}
	})

	t.Run("Register requires Handler implementation", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		entry := TypeOf[TestOrder](KebabNaming)

		err := router.Register(entry)
		if err != ErrNotAHandler {
			t.Errorf("Register() error = %v, want %v", err, ErrNotAHandler)
		}
	})

	t.Run("Register accepts RegistryHandler", func(t *testing.T) {
		router := NewRouter(RouterConfig{})
		handler := NewHandler[TestOrder](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			KebabNaming,
		)

		// Handler implements both TypeEntry and Handler, so it's a RegistryHandler
		err := router.Register(handler.(TypeEntry))
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		entry, ok := router.Lookup("test.order")
		if !ok {
			t.Fatal("Lookup() returned false after Register()")
		}
		if entry.EventType() != "test.order" {
			t.Errorf("EventType() = %q, want %q", entry.EventType(), "test.order")
		}
	})
}

func TestHandlerImplementsTypeEntry(t *testing.T) {
	t.Run("handler implements TypeEntry", func(t *testing.T) {
		h := NewHandler[TestOrder](
			func(ctx context.Context, msg *Message) ([]*Message, error) {
				return nil, nil
			},
			KebabNaming,
		)

		// Verify it implements TypeEntry
		entry, ok := h.(TypeEntry)
		if !ok {
			t.Fatal("handler does not implement TypeEntry")
		}

		if entry.EventType() != "test.order" {
			t.Errorf("EventType() = %q, want %q", entry.EventType(), "test.order")
		}

		instance := entry.NewInstance()
		if _, ok := instance.(*TestOrder); !ok {
			t.Errorf("NewInstance() returned %T, want *TestOrder", instance)
		}
	})

	t.Run("commandHandler implements TypeEntry", func(t *testing.T) {
		h := NewCommandHandler(
			func(ctx context.Context, cmd TestOrder) ([]TestOrder, error) {
				return nil, nil
			},
			CommandHandlerConfig{Source: "/test", Naming: KebabNaming},
		)

		// Verify it implements TypeEntry
		entry, ok := h.(TypeEntry)
		if !ok {
			t.Fatal("commandHandler does not implement TypeEntry")
		}

		if entry.EventType() != "test.order" {
			t.Errorf("EventType() = %q, want %q", entry.EventType(), "test.order")
		}

		instance := entry.NewInstance()
		if _, ok := instance.(*TestOrder); !ok {
			t.Errorf("NewInstance() returned %T, want *TestOrder", instance)
		}
	})
}
