// internal/rawingest/align_bits.go
// PURPOSE: Align packed-bit payload into []bool.
// ALLOWED: pure alignment logic
// FORBIDDEN: sockets, memory access, side effects

package raw

// alignBits converts packed bits payload to []bool length=count.
// Packing: LSB-first per byte (bit0 is first).
func alignBits(payload []byte, count uint16) ([]bool, error) {
	n := int(count)
	out := make([]bool, n)

	for i := 0; i < n; i++ {
		byteIndex := i / 8
		bitIndex := uint(i % 8)

		if byteIndex >= len(payload) {
			return nil, ErrRejected
		}

		out[i] = (payload[byteIndex]&(1<<bitIndex)) != 0
	}

	return out, nil
}
