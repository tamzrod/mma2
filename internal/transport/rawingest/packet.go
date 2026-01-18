// internal/transport/rawingest/packet.go
package rawingest

import "MMA2.0/internal/memorycore"

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

	RespOK       = byte(0)
	RespRejected = byte(1)
)
