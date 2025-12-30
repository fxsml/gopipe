package message

import (
	"context"
	"reflect"
)

// TypeEntry provides type information for CloudEvents message handling.
// It handles registration and unmarshaling concerns, separating them from
// message processing (Handler).
type TypeEntry interface {
	// EventType returns the CloudEvents type this entry handles.
	EventType() string

	// NewInstance creates a new instance for unmarshaling input data.
	// Returns a pointer to the type (e.g., *OrderCommand).
	NewInstance() any
}

// TypeRegistry maps CloudEvents types to TypeEntry instances.
// Used by components like UnmarshalPipe to create typed instances.
type TypeRegistry interface {
	// Lookup returns the TypeEntry for the given CE type.
	Lookup(ceType string) (TypeEntry, bool)

	// Register adds a TypeEntry to the registry.
	Register(entry TypeEntry) error
}

// RegistryHandler combines TypeEntry and Handler interfaces.
// This is equivalent to the original Handler interface but with clearer
// separation of concerns. Use this when a handler needs to be both
// registered in a TypeRegistry and process messages.
type RegistryHandler interface {
	TypeEntry
	Handler
}

// HandlerFunc is an adapter to allow ordinary functions to be used as message
// processors. Similar to http.HandlerFunc. It implements only the Handle method,
// providing a simple way to process messages without implementing the full
// Handler interface (which also includes EventType and NewInput for registration).
//
// Use HandlerFunc with Router.Register when you want to separate type
// registration from message processing:
//
//	router.Register(message.TypeOf[OrderCommand](message.KebabNaming), message.HandlerConfig{})
//	// Then set the handler separately (requires future API addition)
type HandlerFunc func(ctx context.Context, msg *Message) ([]*Message, error)

// Handle calls f(ctx, msg).
func (f HandlerFunc) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	return f(ctx, msg)
}

// typeEntry is a generic TypeEntry implementation.
type typeEntry[T any] struct {
	naming NamingStrategy
}

// TypeOf creates a TypeEntry for a Go type using the given naming strategy.
// This is useful for registering types without a full handler implementation.
//
// Example:
//
//	registry.Register(message.TypeOf[OrderCommand](message.KebabNaming))
func TypeOf[T any](naming NamingStrategy) TypeEntry {
	return &typeEntry[T]{naming: naming}
}

func (e *typeEntry[T]) EventType() string {
	var zero T
	return e.naming.TypeName(reflect.TypeOf(zero))
}

func (e *typeEntry[T]) NewInstance() any {
	return new(T)
}

// MapRegistry is a simple map-based TypeRegistry implementation.
// It stores factory functions for creating type instances.
type MapRegistry map[string]func() any

// Lookup returns the TypeEntry for the given CE type.
func (r MapRegistry) Lookup(ceType string) (TypeEntry, bool) {
	factory, ok := r[ceType]
	if !ok {
		return nil, false
	}
	return &factoryEntry{ceType: ceType, factory: factory}, true
}

// Register adds a TypeEntry to the registry.
func (r MapRegistry) Register(entry TypeEntry) error {
	r[entry.EventType()] = entry.NewInstance
	return nil
}

// factoryEntry wraps a factory function as a TypeEntry.
type factoryEntry struct {
	ceType  string
	factory func() any
}

func (e *factoryEntry) EventType() string {
	return e.ceType
}

func (e *factoryEntry) NewInstance() any {
	return e.factory()
}

// Verify interface compliance.
var (
	_ TypeEntry    = (*typeEntry[any])(nil)
	_ TypeEntry    = (*factoryEntry)(nil)
	_ TypeRegistry = (MapRegistry)(nil)
)
