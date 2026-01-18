// internal/rawingest/constants.go
// PURPOSE: Raw Ingest wire-level constants only.
// ALLOWED: constants, enums, fixed sizes
// FORBIDDEN: logic, imports, side effects

package raw

// ---- Wire protocol constants (V1) ----

// Magic bytes: "RI"
const (
	magic0 = 0x52
	magic1 = 0x49
)

// Protocol version
const (
	protocolVersionV1 = 0x01
)

// ---- Memory areas (table selector, NOT Modbus FCs) ----
type MemoryArea uint8

const (
	AreaCoils          MemoryArea = 0x01
	AreaDiscreteInputs MemoryArea = 0x02
	AreaHoldingRegs    MemoryArea = 0x03
	AreaInputRegs      MemoryArea = 0x04
)

// ---- Fixed sizes ----
const (
	v1HeaderSize = 12
	crcSize      = 4
)

// ---- Single-byte responses ----
const (
	ResponseOK       = 0x00
	ResponseRejected = 0x01
)
