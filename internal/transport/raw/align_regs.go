// internal/rawingest/align_regs.go
// PURPOSE: Align register payload into []uint16.
// ALLOWED: pure alignment logic
// FORBIDDEN: sockets, memory access, side effects

package raw

import "encoding/binary"

// alignRegs converts big-endian uint16 sequence to []uint16 length=count.
func alignRegs(payload []byte, count uint16) ([]uint16, error) {
	n := int(count)
	if len(payload) != n*2 {
		return nil, ErrRejected
	}

	out := make([]uint16, n)
	for i := 0; i < n; i++ {
		out[i] = binary.BigEndian.Uint16(payload[i*2 : i*2+2])
	}
	return out, nil
}
