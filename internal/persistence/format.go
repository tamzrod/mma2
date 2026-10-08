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
//	[ header (24 bytes) ]
//	[ segment descriptor * N (16 bytes each) ]
//	[ metadata CRC32 (4 bytes) ]        <- covers header + descriptors only
//	[ block CRC32 * B (4 bytes each) ]  <- one per fixed-size payload block
//	[ payload ]
//
// The metadata CRC never covers the payload or the block CRC table. The payload
// is divided into fixed-size blocks (default 256 bytes) each protected by its
// own CRC32, so a single register or bit-byte update only recomputes the CRC of
// the block(s) it touches. A whole-file rewrite/rehash is never required.
// Ordinary in-place data+CRC writes are identified by CRC but are NOT themselves
// power-loss atomic; whole-snapshot atomicity is only claimed for creation or
// rebuild via temporary file + rename (P05).
const (
	snapshotMagic         = "MMA2PERS"
	snapshotVersion       = uint16(2)
	fixedHeaderSize       = 24
	segmentDescriptorSize = 16
	metadataCRCSize       = 4
	blockCRCEntrySize     = 4
	defaultBlockSize      = uint16(256)
)

// maxBlockCount bounds the block CRC table to a sane size.
const maxBlockCount = 1 << 20

// SegmentLayout is one persisted segment with its resolved file offsets.
type SegmentLayout struct {
	Segment
	Offset uint32 // byte offset of the segment payload within the file
	Length uint32 // payload length in bytes
}

// Layout is the complete fixed-offset layout for one memory identity.
type Layout struct {
	ID             memorycore.MemoryID
	Segments       []SegmentLayout
	BlockSize      uint16
	BlockCRCOffset uint32
	BlockCount     uint32
	PayloadStart   uint32
	TotalSize      uint32
}

// blockCountFor returns the number of fixed-size blocks covering n payload bytes.
func blockCountFor(n uint32, blockSize uint16) uint32 {
	if blockSize == 0 || n == 0 {
		return 0
	}
	bs := uint32(blockSize)
	return (n + bs - 1) / bs
}

// NewLayout computes a deterministic fixed-offset layout from ordered segments.
func NewLayout(id memorycore.MemoryID, segments []Segment) (*Layout, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("persistence: invalid identity: %w", err)
	}

	layouts := make([]SegmentLayout, 0, len(segments))
	var payloadLenTotal uint32
	for i, seg := range segments {
		if seg.Count == 0 {
			return nil, fmt.Errorf("persistence: segment %d has zero count", i)
		}
		length, err := segmentPayloadLen(seg)
		if err != nil {
			return nil, err
		}
		layouts = append(layouts, SegmentLayout{Segment: seg, Length: uint32(length)})
		payloadLenTotal += uint32(length)
	}

	blockSize := defaultBlockSize
	metaEnd := uint32(fixedHeaderSize + len(segments)*segmentDescriptorSize + metadataCRCSize)

	// The block CRC table size depends only on the total payload length, so the
	// payload start can be computed directly.
	blocks := blockCountFor(payloadLenTotal, blockSize)
	if blocks > maxBlockCount {
		return nil, fmt.Errorf("persistence: snapshot too large for block CRC table (%d blocks)", blocks)
	}
	blockCRCOffset := metaEnd
	payloadStart := blockCRCOffset + blocks*blockCRCEntrySize

	offset := payloadStart
	for i := range layouts {
		layouts[i].Offset = offset
		offset += layouts[i].Length
	}

	return &Layout{
		ID:             id,
		Segments:       layouts,
		BlockSize:      blockSize,
		BlockCRCOffset: blockCRCOffset,
		BlockCount:     blocks,
		PayloadStart:   payloadStart,
		TotalSize:      offset,
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
	return (int(n) + 7) / 8
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

// blockCRCPos returns the file offset of block index i's CRC entry.
func (l *Layout) blockCRCPos(i uint32) uint32 {
	return l.BlockCRCOffset + i*blockCRCEntrySize
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

// EncodeMetadata serialises the header, descriptors and metadata CRC.
// The metadata CRC covers only the header and descriptors.
func (l *Layout) EncodeMetadata() []byte {
	meta := make([]byte, l.BlockCRCOffset)

	copy(meta[0:8], snapshotMagic)
	binary.BigEndian.PutUint16(meta[8:10], snapshotVersion)
	binary.BigEndian.PutUint16(meta[10:12], l.ID.Port)
	binary.BigEndian.PutUint16(meta[12:14], l.ID.UnitID)
	binary.BigEndian.PutUint16(meta[14:16], uint16(len(l.Segments)))
	binary.BigEndian.PutUint16(meta[16:18], l.BlockSize)
	binary.BigEndian.PutUint16(meta[18:20], 0) // reserved
	binary.BigEndian.PutUint32(meta[20:24], l.PayloadStart)

	for i, seg := range l.Segments {
		base := fixedHeaderSize + i*segmentDescriptorSize
		meta[base] = byte(seg.Area)
		meta[base+1] = 0
		binary.BigEndian.PutUint16(meta[base+2:base+4], seg.Start)
		binary.BigEndian.PutUint16(meta[base+4:base+6], seg.Count)
		binary.BigEndian.PutUint16(meta[base+6:base+8], 0)
		binary.BigEndian.PutUint32(meta[base+8:base+12], seg.Offset)
		binary.BigEndian.PutUint32(meta[base+12:base+16], seg.Length)
	}

	crcPos := fixedHeaderSize + len(l.Segments)*segmentDescriptorSize
	binary.BigEndian.PutUint32(meta[crcPos:crcPos+4], crc32.ChecksumIEEE(meta[:crcPos]))

	return meta
}

// BlockCRCs computes the CRC32 of every payload block in a full snapshot image.
func (l *Layout) BlockCRCs(image []byte) []uint32 {
	payload := image[l.PayloadStart:l.TotalSize]
	out := make([]uint32, l.BlockCount)
	for i := range out {
		out[i] = blockCRC(payload, uint32(i), l.BlockSize)
	}
	return out
}

// blockCRC computes the CRC32 of one block of payload.
func blockCRC(payload []byte, blockIndex uint32, blockSize uint16) uint32 {
	start := uint64(blockIndex) * uint64(blockSize)
	if start >= uint64(len(payload)) {
		return crc32.ChecksumIEEE(nil)
	}
	end := start + uint64(blockSize)
	if end > uint64(len(payload)) {
		end = uint64(len(payload))
	}
	return crc32.ChecksumIEEE(payload[start:end])
}

// EncodeBlockCRCs serialises a block CRC table.
func (l *Layout) EncodeBlockCRCs(crcs []uint32) []byte {
	buf := make([]byte, len(crcs)*blockCRCEntrySize)
	for i, c := range crcs {
		binary.BigEndian.PutUint32(buf[i*blockCRCEntrySize:], c)
	}
	return buf
}

// ParseLayout validates a snapshot image's metadata and returns its layout.
// It rejects bad magic/version, metadata CRC mismatch, malformed descriptors,
// mismatched offsets/lengths, truncated payload, and any payload block whose
// stored CRC32 does not match its data.
func ParseLayout(data []byte) (*Layout, error) {
	if len(data) < fixedHeaderSize+metadataCRCSize {
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
	blockSize := binary.BigEndian.Uint16(data[16:18])
	payloadStart := binary.BigEndian.Uint32(data[20:24])

	if blockSize == 0 {
		return nil, fmt.Errorf("persistence: block size is zero")
	}

	metaCRCpos := fixedHeaderSize + segCount*segmentDescriptorSize
	if len(data) < metaCRCpos+metadataCRCSize {
		return nil, fmt.Errorf("persistence: snapshot truncated before metadata CRC")
	}
	wantMetaCRC := binary.BigEndian.Uint32(data[metaCRCpos : metaCRCpos+metadataCRCSize])
	if got := crc32.ChecksumIEEE(data[:metaCRCpos]); got != wantMetaCRC {
		return nil, fmt.Errorf("persistence: snapshot metadata checksum mismatch")
	}

	id := memorycore.MemoryID{Port: port, UnitID: unit}
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("persistence: invalid snapshot identity: %w", err)
	}

	metaEnd := uint32(metaCRCpos + metadataCRCSize)
	if payloadStart < metaEnd {
		return nil, fmt.Errorf("persistence: payload start %d before metadata end %d", payloadStart, metaEnd)
	}
	blockCRCOffset := metaEnd
	tableBytes := payloadStart - blockCRCOffset
	if tableBytes%blockCRCEntrySize != 0 {
		return nil, fmt.Errorf("persistence: block CRC table size %d is not a multiple of %d", tableBytes, blockCRCEntrySize)
	}
	blockCount := tableBytes / blockCRCEntrySize
	if blockCount > maxBlockCount {
		return nil, fmt.Errorf("persistence: implausible block CRC count %d", blockCount)
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

	totalSize := expectOffset
	if uint32(len(data)) < totalSize {
		return nil, fmt.Errorf("persistence: snapshot payload truncated: file=%d want=%d", len(data), totalSize)
	}

	layout := &Layout{
		ID:             id,
		Segments:       segments,
		BlockSize:      blockSize,
		BlockCRCOffset: blockCRCOffset,
		BlockCount:     blockCount,
		PayloadStart:   payloadStart,
		TotalSize:      totalSize,
	}

	wantBlocks := blockCountFor(totalSize-payloadStart, blockSize)
	if wantBlocks != blockCount {
		return nil, fmt.Errorf("persistence: block CRC table has %d entries, need %d", blockCount, wantBlocks)
	}

	if err := layout.VerifyBlockCRCs(data[:totalSize]); err != nil {
		return nil, err
	}

	return layout, nil
}

// VerifyBlockCRCs checks every stored block CRC against the payload bytes.
func (l *Layout) VerifyBlockCRCs(image []byte) error {
	if uint32(len(image)) < l.TotalSize {
		return fmt.Errorf("persistence: image shorter than layout total size")
	}
	payload := image[l.PayloadStart:l.TotalSize]
	for i := uint32(0); i < l.BlockCount; i++ {
		pos := l.blockCRCPos(i)
		stored := binary.BigEndian.Uint32(image[pos : pos+blockCRCEntrySize])
		if got := blockCRC(payload, i, l.BlockSize); got != stored {
			return fmt.Errorf("persistence: block %d CRC mismatch", i)
		}
	}
	return nil
}

// BuildSnapshot assembles a complete snapshot file image from a layout and a
// payload provider. The provider returns the raw payload bytes for a segment
// (packed bits, or big-endian registers), and must return exactly Length bytes.
func (l *Layout) BuildSnapshot(payload func(SegmentLayout) ([]byte, error)) ([]byte, error) {
	out := make([]byte, l.TotalSize)
	copy(out[:l.PayloadStart], l.EncodeMetadata())

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

	copy(out[l.BlockCRCOffset:l.PayloadStart], l.EncodeBlockCRCs(l.BlockCRCs(out)))
	return out, nil
}
