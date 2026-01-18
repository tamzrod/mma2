// internal/memorycore/memory_id.go
package memorycore

type MemoryID struct {
	Port   string
	UnitID uint16
}

func (id MemoryID) Validate() error {
	if id.Port == "" {
		return ErrEmptyPort
	}
	if id.UnitID == 0 {
		return ErrUnitIDZero
	}
	return nil
}
