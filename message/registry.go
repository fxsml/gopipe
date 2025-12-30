package message

// TypeRegistry creates typed instances for unmarshaling.
type TypeRegistry interface {
	NewInstance(ceType string) any // nil if unknown type
}

// FactoryMap is a simple TypeRegistry for standalone use.
type FactoryMap map[string]func() any

func (m FactoryMap) NewInstance(ceType string) any {
	if f, ok := m[ceType]; ok {
		return f()
	}
	return nil
}

var _ TypeRegistry = (FactoryMap)(nil)
