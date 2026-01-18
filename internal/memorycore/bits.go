// internal/memorycore/bits.go
package memorycore

// bytesForBits returns the number of bytes required
// to store n bits (LSB-first).
func bytesForBits(n uint16) int {
	if n == 0 {
		return 0
	}
	return int((n + 7) / 8)
}

// copyBits copies packed bit data from src into dst.
// start is the starting bit offset in dst.
// count is the number of bits to copy.
func copyBits(dst []byte, src []byte, start uint16, count uint16) {
	for i := uint16(0); i < count; i++ {
		dstBit := start + i

		dstByte := dstBit / 8
		dstMask := byte(1 << (dstBit % 8))

		srcByte := i / 8
		srcMask := byte(1 << (i % 8))

		if int(dstByte) >= len(dst) || int(srcByte) >= len(src) {
			return
		}

		if src[srcByte]&srcMask != 0 {
			dst[dstByte] |= dstMask
		} else {
			dst[dstByte] &^= dstMask
		}
	}
}

// writeBits writes packed bits from src into dst.
// start is the starting bit offset in dst.
// count is the number of bits to write.
func writeBits(dst []byte, start uint16, count uint16, src []byte) {
	for i := uint16(0); i < count; i++ {
		dstBit := start + i

		dstByte := dstBit / 8
		dstMask := byte(1 << (dstBit % 8))

		srcByte := i / 8
		srcMask := byte(1 << (i % 8))

		if int(dstByte) >= len(dst) || int(srcByte) >= len(src) {
			return
		}

		if src[srcByte]&srcMask != 0 {
			dst[dstByte] |= dstMask
		} else {
			dst[dstByte] &^= dstMask
		}
	}
}
