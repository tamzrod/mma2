package persistence

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"mma2/internal/memorycore"
)

func sampleSegments() []Segment {
	return []Segment{
		{Area: memorycore.AreaCoils, Start: 0, Count: 12},
		{Area: memorycore.AreaHoldingRegs, Start: 4, Count: 3},
	}
}

func TestNewLayoutOffsets(t *testing.T) {
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	l, err := NewLayout(id, sampleSegments())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// header 24 + 2 descriptors*16 + crc 4 = 60
	if l.PayloadStart != 60 {
		t.Fatalf("payload start = %d, want 60", l.PayloadStart)
	}
	if len(l.Segments) != 2 {
		t.Fatalf("segments = %d", len(l.Segments))
	}
	coils := l.Segments[0]
	if coils.Length != 2 || coils.Offset != 60 { // ceil(12/8)=2
		t.Fatalf("coils layout wrong: %+v", coils)
	}
	regs := l.Segments[1]
	if regs.Length != 6 || regs.Offset != 62 { // 3*2
		t.Fatalf("regs layout wrong: %+v", regs)
	}
	if l.TotalSize != 68 {
		t.Fatalf("total size = %d, want 68", l.TotalSize)
	}
}

func TestLayoutOffsetHelpers(t *testing.T) {
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}

	off, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 5)
	if err != nil {
		t.Fatalf("RegisterOffset: %v", err)
	}
	if off != 62+2 { // start 4 -> address 5 is index 1 -> +2 bytes
		t.Fatalf("register offset = %d, want 64", off)
	}
	if _, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 2); err == nil {
		t.Fatal("address below persisted range accepted")
	}
	if _, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 7); err == nil {
		t.Fatal("address above persisted range accepted")
	}
	if _, err := l.RegisterOffset(memorycore.AreaCoils, 0); err == nil {
		t.Fatal("non-register area accepted for RegisterOffset")
	}

	byteOff, bit, err := l.BitByteOffset(memorycore.AreaCoils, 9)
	if err != nil {
		t.Fatalf("BitByteOffset: %v", err)
	}
	if byteOff != 60+1 || bit != 1 {
		t.Fatalf("bit offset = %d/%d, want 61/1", byteOff, bit)
	}
	if _, _, err := l.BitByteOffset(memorycore.AreaCoils, 12); err == nil {
		t.Fatal("address outside bit range accepted")
	}
	if _, _, err := l.BitByteOffset(memorycore.AreaHoldingRegs, 4); err == nil {
		t.Fatal("non-bit area accepted for BitByteOffset")
	}
}

func TestEncodeParseRoundtrip(t *testing.T) {
	id := memorycore.MemoryID{Port: 503, UnitID: 2}
	l, err := NewLayout(id, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}

	coilsPayload := []byte{0xAB, 0x0F} // packed bits, unused high nibble ignored
	regsPayload := []byte{0x00, 0x01, 0x00, 0x02, 0x12, 0x34}
	image, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) {
		if seg.Area == memorycore.AreaCoils {
			return coilsPayload, nil
		}
		return regsPayload, nil
	})
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if uint32(len(image)) != l.TotalSize {
		t.Fatalf("image size = %d, want %d", len(image), l.TotalSize)
	}

	parsed, err := ParseLayout(image)
	if err != nil {
		t.Fatalf("ParseLayout: %v", err)
	}
	if parsed.ID != id || parsed.TotalSize != l.TotalSize || parsed.PayloadStart != l.PayloadStart {
		t.Fatalf("parsed layout differs: %+v vs %+v", parsed, l)
	}
	if len(parsed.Segments) != len(l.Segments) {
		t.Fatalf("segment count differs")
	}
	for i := range parsed.Segments {
		if parsed.Segments[i] != l.Segments[i] {
			t.Fatalf("segment %d differs: %+v vs %+v", i, parsed.Segments[i], l.Segments[i])
		}
	}
	if got := image[parsed.Segments[0].Offset : parsed.Segments[0].Offset+parsed.Segments[0].Length]; !bytes.Equal(got, coilsPayload) {
		t.Fatalf("coils payload mismatch: %x", got)
	}
	if got := image[parsed.Segments[1].Offset : parsed.Segments[1].Offset+parsed.Segments[1].Length]; !bytes.Equal(got, regsPayload) {
		t.Fatalf("regs payload mismatch: %x", got)
	}
}

func TestParseLayoutRejectsTampering(t *testing.T) {
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	image, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) {
		return make([]byte, seg.Length), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("bad magic", func(t *testing.T) {
		bad := append([]byte(nil), image...)
		bad[0] = 'X'
		if _, err := ParseLayout(bad); err == nil {
			t.Fatal("bad magic accepted")
		}
	})
	t.Run("bad version", func(t *testing.T) {
		bad := append([]byte(nil), image...)
		binary.BigEndian.PutUint16(bad[8:10], 99)
		if _, err := ParseLayout(bad); err == nil {
			t.Fatal("bad version accepted")
		}
	})
	t.Run("header crc mismatch", func(t *testing.T) {
		bad := append([]byte(nil), image...)
		binary.BigEndian.PutUint16(bad[14:16], 1) // wrong segment count -> crc mismatch
		if _, err := ParseLayout(bad); err == nil {
			t.Fatal("corrupt header accepted")
		}
	})
	t.Run("truncated", func(t *testing.T) {
		if _, err := ParseLayout(image[:10]); err == nil {
			t.Fatal("truncated header accepted")
		}
	})
	t.Run("payload truncated", func(t *testing.T) {
		if _, err := ParseLayout(image[:len(image)-1]); err == nil {
			t.Fatal("truncated payload accepted")
		}
	})
	t.Run("identity mismatch is not silent", func(t *testing.T) {
		// Change identity in header then recompute header CRC so only identity differs.
		bad := append([]byte(nil), image...)
		binary.BigEndian.PutUint16(bad[10:12], 999)
		recomputeHeaderCRC(bad, l)
		parsed, err := ParseLayout(bad)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.ID.Port == l.ID.Port {
			t.Fatal("identity change not reflected")
		}
	})
}

// recomputeHeaderCRC rewrites the header CRC after a header mutation, used to
// isolate identity checks in tests.
func recomputeHeaderCRC(image []byte, l *Layout) {
	crcPos := fixedHeaderSize + len(l.Segments)*segmentDescriptorSize
	binary.BigEndian.PutUint32(image[crcPos:crcPos+4], 0)
	sum := crc32.ChecksumIEEE(image[:crcPos])
	binary.BigEndian.PutUint32(image[crcPos:crcPos+4], sum)
}

func TestNewLayoutRejectsInvalidIdentityAndArea(t *testing.T) {
	if _, err := NewLayout(memorycore.MemoryID{}, sampleSegments()); err == nil {
		t.Fatal("zero identity accepted")
	}
	if _, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, []Segment{{Area: memorycore.AreaInvalid, Start: 0, Count: 1}}); err == nil {
		t.Fatal("invalid area accepted")
	}
	if _, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, []Segment{{Area: memorycore.AreaCoils, Start: 0, Count: 0}}); err == nil {
		t.Fatal("zero-count segment accepted")
	}
}
