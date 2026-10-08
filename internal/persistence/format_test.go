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

	// payload: coils ceil(12/8)=2, regs 3*2=6 => 8 bytes => 1 block.
	if l.BlockSize != defaultBlockSize {
		t.Fatalf("block size = %d", l.BlockSize)
	}
	if l.BlockCount != 1 {
		t.Fatalf("block count = %d, want 1", l.BlockCount)
	}
	// metadata: 24 + 2*16 + 4 = 60; CRC table: 1*4 = 4; payload start = 64.
	if l.BlockCRCOffset != 60 || l.PayloadStart != 64 {
		t.Fatalf("offsets wrong: crc=%d payload=%d", l.BlockCRCOffset, l.PayloadStart)
	}
	coils := l.Segments[0]
	if coils.Length != 2 || coils.Offset != 64 {
		t.Fatalf("coils layout wrong: %+v", coils)
	}
	regs := l.Segments[1]
	if regs.Length != 6 || regs.Offset != 66 {
		t.Fatalf("regs layout wrong: %+v", regs)
	}
	if l.TotalSize != 72 {
		t.Fatalf("total size = %d, want 72", l.TotalSize)
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
	if off != 66+2 {
		t.Fatalf("register offset = %d, want 68", off)
	}
	if _, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 2); err == nil {
		t.Fatal("address below persisted range accepted")
	}
	if _, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 7); err == nil {
		t.Fatal("address above persisted range accepted")
	}
	if _, err := l.RegisterOffset(memorycore.AreaCoils, 0); err == nil {
		t.Fatal("non-register area accepted")
	}

	byteOff, bit, err := l.BitByteOffset(memorycore.AreaCoils, 9)
	if err != nil {
		t.Fatalf("BitByteOffset: %v", err)
	}
	if byteOff != 64+1 || bit != 1 {
		t.Fatalf("bit offset = %d/%d, want 65/1", byteOff, bit)
	}
	if _, _, err := l.BitByteOffset(memorycore.AreaCoils, 12); err == nil {
		t.Fatal("address outside bit range accepted")
	}
	if _, _, err := l.BitByteOffset(memorycore.AreaHoldingRegs, 4); err == nil {
		t.Fatal("non-bit area accepted")
	}
}

func TestEncodeParseRoundtrip(t *testing.T) {
	id := memorycore.MemoryID{Port: 503, UnitID: 2}
	l, err := NewLayout(id, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	coilsPayload := []byte{0xAB, 0x0F}
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
	if !sameLayout(parsed, l) {
		t.Fatalf("parsed layout differs: %+v vs %+v", parsed, l)
	}
	if got := image[parsed.Segments[0].Offset : parsed.Segments[0].Offset+parsed.Segments[0].Length]; !bytes.Equal(got, coilsPayload) {
		t.Fatalf("coils payload mismatch: %x", got)
	}
	if got := image[parsed.Segments[1].Offset : parsed.Segments[1].Offset+parsed.Segments[1].Length]; !bytes.Equal(got, regsPayload) {
		t.Fatalf("regs payload mismatch: %x", got)
	}
}

func TestBlockCRCsDetectPayloadCorruption(t *testing.T) {
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	image, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) { return make([]byte, seg.Length), nil })
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt one payload byte; parse must reject via block CRC.
	bad := append([]byte(nil), image...)
	bad[l.PayloadStart] ^= 0xFF
	if _, err := ParseLayout(bad); err == nil {
		t.Fatal("payload corruption not detected")
	}
}

func TestBlockCRCsPerBlockGranularity(t *testing.T) {
	// 600 register bytes => 3 blocks of 256/256/88.
	segs := []Segment{{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 300}}
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, segs)
	if err != nil {
		t.Fatal(err)
	}
	if l.BlockCount != 3 {
		t.Fatalf("block count = %d, want 3", l.BlockCount)
	}
	image, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) { return make([]byte, seg.Length), nil })
	if err != nil {
		t.Fatal(err)
	}
	crcs := l.BlockCRCs(image)
	if len(crcs) != 3 {
		t.Fatalf("crc count = %d", len(crcs))
	}

	// Corrupt a byte in block 1 only and recompute nothing: stored CRC for
	// block 1 mismatches, block 0 and 2 stay valid.
	bad := append([]byte(nil), image...)
	bad[l.PayloadStart+256] ^= 0x01
	if err := l.VerifyBlockCRCs(bad); err == nil {
		t.Fatal("expected block 1 CRC mismatch")
	}
}

func TestParseLayoutRejectsTampering(t *testing.T) {
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	image, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) { return make([]byte, seg.Length), nil })
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
	t.Run("metadata crc mismatch", func(t *testing.T) {
		bad := append([]byte(nil), image...)
		bad[14] ^= 0xFF
		if _, err := ParseLayout(bad); err == nil {
			t.Fatal("corrupt metadata accepted")
		}
	})
	t.Run("truncated header", func(t *testing.T) {
		if _, err := ParseLayout(image[:10]); err == nil {
			t.Fatal("truncated header accepted")
		}
	})
	t.Run("payload truncated", func(t *testing.T) {
		if _, err := ParseLayout(image[:len(image)-1]); err == nil {
			t.Fatal("truncated payload accepted")
		}
	})
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

func TestMetadataCRCIgnoresPayloadChanges(t *testing.T) {
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	image, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) { return make([]byte, seg.Length), nil })
	if err != nil {
		t.Fatal(err)
	}
	// The metadata CRC must be identical regardless of payload bytes.
	metaCRC := binary.BigEndian.Uint32(image[l.BlockCRCOffset-4 : l.BlockCRCOffset])
	image2, _ := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) {
		b := make([]byte, seg.Length)
		for i := range b {
			b[i] = 0xAA
		}
		return b, nil
	})
	metaCRC2 := binary.BigEndian.Uint32(image2[l.BlockCRCOffset-4 : l.BlockCRCOffset])
	if metaCRC != metaCRC2 {
		t.Fatal("metadata CRC depends on payload (would force whole-file rehash)")
	}
	if crc32.ChecksumIEEE(image[:l.BlockCRCOffset-4]) != metaCRC {
		t.Fatal("metadata CRC not self-consistent")
	}
}

func TestBytesForBitsUpperBoundary(t *testing.T) {
    for _,tc := range []struct{ bits uint16; bytes int }{
        {1,1},{7,1},{8,1},{9,2},{65528,8191},{65535,8192},
    } {
        if got:=bytesForBits(tc.bits);got!=tc.bytes {
            t.Errorf("bytesForBits(%d)=%d, want %d",tc.bits,got,tc.bytes)
        }
    }
}
