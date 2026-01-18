// internal/rawingest/types.go
// PURPOSE: Raw Ingest interfaces only.
// ALLOWED: interface and type definitions
// FORBIDDEN: logic, imports, default implementations

package raw

// RawWritableMemory is the ONLY surface Raw Ingest can touch.
// Bounds, atomicity, and locking are enforced by the memory itself.
type RawWritableMemory interface {
	WriteCoils(address uint16, values []bool) error
	WriteDiscreteInputs(address uint16, values []bool) error
	WriteHoldingRegisters(address uint16, values []uint16) error
	WriteInputRegisters(address uint16, values []uint16) error
}

// MemoryResolver resolves a memory instance by numeric ID.
// Raw Ingest does not know how memories are created or routed.
type MemoryResolver interface {
	ResolveMemoryByID(id uint16) (RawWritableMemory, bool)
}
