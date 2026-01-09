package message

// InputRegistry creates typed instances for unmarshaling.
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
