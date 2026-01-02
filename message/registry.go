package message

// InputRegistry creates typed instances for unmarshaling.
type InputRegistry interface {
	NewInput(ceType string) any // nil if unknown type
}

// FactoryMap is a simple InputRegistry for standalone use.
type FactoryMap map[string]func() any

func (m FactoryMap) NewInput(ceType string) any {
	if f, ok := m[ceType]; ok {
		return f()
	}
	return nil
}

var _ InputRegistry = (FactoryMap)(nil)
