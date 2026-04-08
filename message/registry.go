package message

// InputRegistry creates typed instances for unmarshaling.
// Implementations return nil when no Go type is available for the given
// event type. Callers (such as UnmarshalPipe) must treat a nil return as
// an unresolvable type and handle it gracefully — typically by returning
// ErrUnknownType rather than attempting to unmarshal.
type InputRegistry interface {
	NewInput(eventType string) any // nil if unknown type
}

// FactoryMap is a simple InputRegistry for standalone use.
type FactoryMap map[string]func() any

func (m FactoryMap) NewInput(eventType string) any {
	if f, ok := m[eventType]; ok {
		return f()
	}
	return nil
}

var _ InputRegistry = (FactoryMap)(nil)
