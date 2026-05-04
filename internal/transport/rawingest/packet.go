// internal/transport/rawingest/packet.go
package rawingest

import "mma2/internal/memorycore"

// Packet is a single raw-ingest write primitive.
type Packet struct {
	Port   uint16
	UnitID uint16

	Area    memorycore.Area
	Address uint16
	Count   uint16

	// Payload encoding:
	// - Bit areas: packed bits (LSB-first), bytes = ceil(count/8)
	// - Reg areas: big-endian uint16 words, bytes = count*2
	Payload []byte
}

const (
	Magic0 = byte('R')
	Magic1 = byte('I')

	Version1 = byte(0x01)

	RespOK = byte(0x00)

	RespInvalidMagic   = byte(0x10)
	RespInvalidVersion = byte(0x11)
	RespInvalidArea    = byte(0x12)
	RespInvalidCount   = byte(0x13)
	RespInvalidLength  = byte(0x14)

	RespMemoryNotFound = byte(0x20)
	RespOutOfBounds    = byte(0x21)

	RespInternalError = byte(0x30)
)
