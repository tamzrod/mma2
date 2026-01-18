// internal/memorycore/memory_id.go
package memorycore

// MemoryID uniquely identifies a memory instance.
// Architectural rule (LOCKED):
//   MemoryID = (Port:uint16, UnitID:uint16)
type MemoryID struct {
	Port   uint16
	UnitID uint16
}

func (id MemoryID) Validate() error {
	// Port and UnitID are protocol-derived.
	// Zero is invalid for both.
	if id.Port == 0 {
		return ErrEmptyPort
	}
	if id.UnitID == 0 {
		return ErrUnitIDZero
	}
	return nil
}
