package persistence

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"

	"mma2/internal/memorycore"
)

// Fixed-layout binary snapshot format.
//
// File structure:
//
//	[ fixed header (24 bytes) ][ segment descriptor * N (16 bytes each) ][ header CRC32 (4 bytes) ][ payload ]
//
// The header CRC32 covers only the header and descriptors, never the payload.
// This lets a single register or bit-byte be updated in place (via a positional
// WriteAt) without rewriting or rehashing the whole file, as required by the
// fixed-offset contract. Ordinary in-place writes are NOT power-loss atomic;
// whole-snapshot atomicity is only claimed for initial creation/rebuild via
// temporary file + rename (P05).
const (
	snapshotMagic         = "MMA2PERS"
	snapshotVersion       = uint16(1)
	fixedHeaderSize       = 24
	segmentDescriptorSize = 16
	headerCRCSize         = 4
)

// SegmentLayout is one persisted segment with its resolved file offsets.
type SegmentLayout struct {
	Segment
	Offset uint32 // byte offset of the segment payload within the file
	Length uint32 // payload length in bytes
}

// Layout is the complete fixed-offset layout for one memory identity.
type Layout struct {
	ID           memorycore.MemoryID
	Segments     []SegmentLayout
	PayloadStart uint32
	TotalSize    uint32
}

// NewLayout computes a deterministic fixed-offset layout from ordered segments.
func NewLayout(id memorycore.MemoryID, segments []Segment) (*Layout, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("persistence: invalid identity: %w", err)
	}

	headerEnd := uint32(fixedHeaderSize + len(segments)*segmentDescriptorSize + headerCRCSize)
	offset := headerEnd

	layouts := make([]SegmentLayout, 0, len(segments))
	for i, seg := range segments {
		if seg.Count == 0 {
			return nil, fmt.Errorf("persistence: segment %d has zero count", i)
		}
		length, err := segmentPayloadLen(seg)
		if err != nil {
			return nil, err
		}
		layouts = append(layouts, SegmentLayout{
			Segment: seg,
			Offset:  offset,
			Length:  uint32(length),
		})
		offset += uint32(length)
	}

	return &Layout{
		ID:           id,
		Segments:     layouts,
		PayloadStart: headerEnd,
		TotalSize:    offset,
	}, nil
}

// segmentPayloadLen returns the byte length of a segment's payload:
// ceil(count/8) for bit-areas, count*2 for register-areas.
func segmentPayloadLen(seg Segment) (int, error) {
	switch {
	case seg.Area.IsBitArea():
		return bytesForBits(seg.Count), nil
	case seg.Area.IsRegArea():
		return int(seg.Count) * 2, nil
	default:
		return 0, fmt.Errorf("persistence: invalid area %d", seg.Area)
	}
}

func bytesForBits(n uint16) int {
	if n == 0 {
		return 0
	}
	return int((n + 7) / 8)
}

// segmentFor returns the layout for an area, if present.
func (l *Layout) segmentFor(area memorycore.Area) (SegmentLayout, bool) {
	for _, s := range l.Segments {
		if s.Area == area {
			return s, true
		}
	}
	return SegmentLayout{}, false
}

// RegisterOffset returns the file offset for a 2-byte register write and
// verifies the address is inside the persisted register segment.
func (l *Layout) RegisterOffset(area memorycore.Area, address uint16) (uint32, error) {
	if !area.IsRegArea() {
		return 0, fmt.Errorf("persistence: area %s is not a register area", area)
	}
	seg, ok := l.segmentFor(area)
	if !ok {
		return 0, fmt.Errorf("persistence: %s is not persisted for this identity", area)
	}
	if !contains(seg.Segment, address, 1) {
		return 0, fmt.Errorf("persistence: address %d outside persisted %s range", address, area)
	}
	return seg.Offset + uint32(address-seg.Start)*2, nil
}

// BitByteOffset returns the file offset of the byte containing a bit address,
// and the bit index within that byte, verifying the address is persisted.
func (l *Layout) BitByteOffset(area memorycore.Area, address uint16) (uint32, uint8, error) {
	if !area.IsBitArea() {
		return 0, 0, fmt.Errorf("persistence: area %s is not a bit area", area)
	}
	seg, ok := l.segmentFor(area)
	if !ok {
		return 0, 0, fmt.Errorf("persistence: %s is not persisted for this identity", area)
	}
	if !contains(seg.Segment, address, 1) {
		return 0, 0, fmt.Errorf("persistence: address %d outside persisted %s range", address, area)
	}
	bit := uint16(address - seg.Start)
	return seg.Offset + uint32(bit/8), uint8(bit % 8), nil
}

func contains(seg Segment, address, count uint16) bool {
	end := uint32(seg.Start) + uint32(seg.Count)
	reqEnd := uint32(address) + uint32(count)
	return uint32(address) >= uint32(seg.Start) && reqEnd <= end
}

// EncodeHeader serialises the fixed header, descriptors and CRC for this layout.
func (l *Layout) EncodeHeader() []byte {
	header := make([]byte, fixedHeaderSize+len(l.Segments)*segmentDescriptorSize+headerCRCSize)

	copy(header[0:8], snapshotMagic)
	binary.BigEndian.PutUint16(header[8:10], snapshotVersion)
	binary.BigEndian.PutUint16(header[10:12], l.ID.Port)
	binary.BigEndian.PutUint16(header[12:14], l.ID.UnitID)
	binary.BigEndian.PutUint16(header[14:16], uint16(len(l.Segments)))
	binary.BigEndian.PutUint32(header[16:20], l.PayloadStart)
	binary.BigEndian.PutUint32(header[20:24], 0) // reserved flags

	for i, seg := range l.Segments {
		base := fixedHeaderSize + i*segmentDescriptorSize
		header[base] = byte(seg.Area)
		header[base+1] = 0
		binary.BigEndian.PutUint16(header[base+2:base+4], seg.Start)
		binary.BigEndian.PutUint16(header[base+4:base+6], seg.Count)
		binary.BigEndian.PutUint16(header[base+6:base+8], 0)
		binary.BigEndian.PutUint32(header[base+8:base+12], seg.Offset)
		binary.BigEndian.PutUint32(header[base+12:base+16], seg.Length)
	}

	crcPos := fixedHeaderSize + len(l.Segments)*segmentDescriptorSize
	binary.BigEndian.PutUint32(header[crcPos:crcPos+4], crc32.ChecksumIEEE(header[:crcPos]))

	return header
}

// ParseLayout validates a snapshot file's header and returns its layout.
// It rejects bad magic/version, checksum mismatch, malformed descriptors, and
// overlapping or out-of-order payload regions.
func ParseLayout(data []byte) (*Layout, error) {
	if len(data) < fixedHeaderSize+headerCRCSize {
		return nil, fmt.Errorf("persistence: snapshot shorter than header")
	}
	if string(data[0:8]) != snapshotMagic {
		return nil, fmt.Errorf("persistence: bad snapshot magic")
	}

	version := binary.BigEndian.Uint16(data[8:10])
	if version != snapshotVersion {
		return nil, fmt.Errorf("persistence: unsupported snapshot version %d", version)
	}

	port := binary.BigEndian.Uint16(data[10:12])
	unit := binary.BigEndian.Uint16(data[12:14])
	segCount := int(binary.BigEndian.Uint16(data[14:16]))
	payloadStart := binary.BigEndian.Uint32(data[16:20])

	headerLen := fixedHeaderSize + segCount*segmentDescriptorSize + headerCRCSize
	if len(data) < headerLen {
		return nil, fmt.Errorf("persistence: snapshot truncated before descriptors")
	}
	if payloadStart != uint32(headerLen) {
		return nil, fmt.Errorf("persistence: payload start mismatch: header=%d file=%d", headerLen, payloadStart)
	}

	crcPos := fixedHeaderSize + segCount*segmentDescriptorSize
	wantCRC := binary.BigEndian.Uint32(data[crcPos : crcPos+4])
	if got := crc32.ChecksumIEEE(data[:crcPos]); got != wantCRC {
		return nil, fmt.Errorf("persistence: snapshot header checksum mismatch")
	}

	id := memorycore.MemoryID{Port: port, UnitID: unit}
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("persistence: invalid snapshot identity: %w", err)
	}

	segments := make([]SegmentLayout, 0, segCount)
	expectOffset := payloadStart
	for i := 0; i < segCount; i++ {
		base := fixedHeaderSize + i*segmentDescriptorSize
		area := memorycore.Area(data[base])
		cnt := binary.BigEndian.Uint16(data[base+4 : base+6])
		offset := binary.BigEndian.Uint32(data[base+8 : base+12])
		length := binary.BigEndian.Uint32(data[base+12 : base+16])

		seg := Segment{Area: area, Start: binary.BigEndian.Uint16(data[base+2 : base+4]), Count: cnt}
		if cnt == 0 {
			return nil, fmt.Errorf("persistence: descriptor %d has zero count", i)
		}
		wantLen, err := segmentPayloadLen(seg)
		if err != nil {
			return nil, fmt.Errorf("persistence: descriptor %d: %w", i, err)
		}
		if int(length) != wantLen {
			return nil, fmt.Errorf("persistence: descriptor %d length mismatch: file=%d want=%d", i, length, wantLen)
		}
		if offset != expectOffset {
			return nil, fmt.Errorf("persistence: descriptor %d offset mismatch: file=%d want=%d", i, offset, expectOffset)
		}
		expectOffset = offset + length

		segments = append(segments, SegmentLayout{Segment: seg, Offset: offset, Length: length})
	}

	if uint32(len(data)) < expectOffset {
		return nil, fmt.Errorf("persistence: snapshot payload truncated: file=%d want=%d", len(data), expectOffset)
	}

	return &Layout{
		ID:           id,
		Segments:     segments,
		PayloadStart: payloadStart,
		TotalSize:    expectOffset,
	}, nil
}

// BuildSnapshot assembles a complete snapshot file image from a layout and a
// payload provider. The provider returns the raw payload bytes for a segment
// (packed bits, or big-endian registers), and must return exactly Length bytes.
func (l *Layout) BuildSnapshot(payload func(SegmentLayout) ([]byte, error)) ([]byte, error) {
	out := make([]byte, l.TotalSize)
	copy(out[:l.PayloadStart], l.EncodeHeader())

	for _, seg := range l.Segments {
		data, err := payload(seg)
		if err != nil {
			return nil, err
		}
		if uint32(len(data)) != seg.Length {
			return nil, fmt.Errorf("persistence: segment %s payload length %d, want %d", seg.Area, len(data), seg.Length)
		}
		copy(out[seg.Offset:seg.Offset+seg.Length], data)
	}

	return out, nil
}
