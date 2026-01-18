// internal/transport/raw/memory_adapter.go
package raw

import "MMA2.0/internal/runtime"

// memoryAdapter exposes runtime memory through RawWritableMemory.
type memoryAdapter struct {
	mem *runtime.Memory
}

func (m *memoryAdapter) WriteCoils(addr uint16, values []bool) error {
	for i, v := range values {
		m.mem.Coils.Data[int(addr)+i] = boolToUint16(v)
	}
	return nil
}

func (m *memoryAdapter) WriteDiscreteInputs(addr uint16, values []bool) error {
	for i, v := range values {
		m.mem.DiscreteInputs.Data[int(addr)+i] = boolToUint16(v)
	}
	return nil
}

func (m *memoryAdapter) WriteHoldingRegisters(addr uint16, values []uint16) error {
	for i, v := range values {
		m.mem.HoldingRegs.Data[int(addr)+i] = v
	}
	return nil
}

func (m *memoryAdapter) WriteInputRegisters(addr uint16, values []uint16) error {
	for i, v := range values {
		m.mem.InputRegs.Data[int(addr)+i] = v
	}
	return nil
}

func boolToUint16(v bool) uint16 {
	if v {
		return 1
	}
	return 0
}
