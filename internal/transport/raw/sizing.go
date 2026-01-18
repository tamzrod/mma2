// internal/rawingest/sizing.go
// PURPOSE: Compute payload size based on memory area and count.
// ALLOWED: arithmetic only
// FORBIDDEN: packet parsing, sockets, memory access

package raw

// payloadSize returns the expected payload size in bytes
// for a given memory area and element count.
func payloadSize(area MemoryArea, count uint16) (int, error) {
	switch area {
	case AreaCoils, AreaDiscreteInputs:
		// Packed bits, LSB-first, ceil(count / 8)
		return int((count + 7) / 8), nil

	case AreaHoldingRegs, AreaInputRegs:
		// uint16 per register
		return int(count) * 2, nil

	default:
		return 0, ErrRejected
	}
}
