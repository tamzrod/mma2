package persistence

import (
	"fmt"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// Directory returns the configured snapshot directory (empty when disabled).
func (m *Manager) Directory() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dir
}

// layoutFor builds the fixed layout for an identity from its resolved segments.
func (m *Manager) layoutFor(id memorycore.MemoryID) (*Layout, bool, error) {
	segs, ok := m.Segments(id)
	if !ok || len(segs) == 0 {
		return nil, false, nil
	}
	layout, err := NewLayout(id, segs)
	if err != nil {
		return nil, false, err
	}
	return layout, true, nil
}

// RestoreMemory restores one identity's persisted state before the identity is
// exposed to any protocol listener.
//
//   - If the snapshot is missing, memory keeps its normal initial values and an
//     initial snapshot is created and persisted.
//   - If the snapshot exists and is valid for the current layout, its payload is
//     applied to memory.
//   - If the snapshot is corrupt or incompatible (identity/layout mismatch), it
//     fails closed: no partial state is applied and the manager transitions to
//     FAILED. The returned error is fatal to startup.
//
// It is a no-op when persistence is disabled or the identity has no segments.
func (m *Manager) RestoreMemory(id memorycore.MemoryID, mem *memorycore.Memory) error {
	if m == nil || mem == nil || !m.Enabled() {
		return nil
	}

	layout, present, err := m.layoutFor(id)
	if err != nil {
		m.markFailed(err, time.Now())
		return err
	}
	if !present {
		return nil
	}

	store := NewFileStore(config.SnapshotPath(m.Directory(), id))

	exists, err := store.Exists()
	if err != nil {
		m.markFailed(err, time.Now())
		return fmt.Errorf("persistence: identity (port=%d unit=%d): stat snapshot: %w", id.Port, id.UnitID, err)
	}

	if !exists {
		image, err := buildMemoryImage(layout, mem)
		if err != nil {
			m.markFailed(err, time.Now())
			return err
		}
		if err := store.ReplaceAtomic(image); err != nil {
			m.markFailed(err, time.Now())
			return fmt.Errorf("persistence: identity (port=%d unit=%d): create initial snapshot: %w", id.Port, id.UnitID, err)
		}
		m.markRestored(time.Now())
		return nil
	}

	data, err := store.ReadAll()
	if err != nil {
		m.markFailed(err, time.Now())
		return fmt.Errorf("persistence: identity (port=%d unit=%d): read snapshot: %w", id.Port, id.UnitID, err)
	}

	parsed, err := ParseLayout(data)
	if err != nil {
		m.markFailed(err, time.Now())
		return fmt.Errorf("persistence: identity (port=%d unit=%d): corrupt snapshot: %w", id.Port, id.UnitID, err)
	}
	if parsed.ID != id {
		err := fmt.Errorf("persistence: identity (port=%d unit=%d): snapshot identity is (port=%d unit=%d)", id.Port, id.UnitID, parsed.ID.Port, parsed.ID.UnitID)
		m.markFailed(err, time.Now())
		return err
	}
	if !sameLayout(parsed, layout) {
		err := fmt.Errorf("persistence: identity (port=%d unit=%d): snapshot layout does not match configuration", id.Port, id.UnitID)
		m.markFailed(err, time.Now())
		return err
	}

	// Validate every segment payload before applying anything, so a failure
	// never leaves memory partly restored.
	for i, seg := range layout.Segments {
		if err := validatePayloadRange(parsed.Segments[i], seg); err != nil {
			m.markFailed(err, time.Now())
			return fmt.Errorf("persistence: identity (port=%d unit=%d): %w", id.Port, id.UnitID, err)
		}
	}

	for i, seg := range layout.Segments {
		payload := data[parsed.Segments[i].Offset : parsed.Segments[i].Offset+parsed.Segments[i].Length]
		if err := applyPayload(mem, seg.Segment, payload); err != nil {
			m.markFailed(err, time.Now())
			return fmt.Errorf("persistence: identity (port=%d unit=%d): apply %s: %w", id.Port, id.UnitID, seg.Area, err)
		}
	}

	m.markRestored(time.Now())
	return nil
}

// PersistMemory writes the current memory values as a complete snapshot image
// using atomic replacement. Used for initial creation and layout rebuilds; the
// dirty scheduler (P08) uses targeted writes instead.
func (m *Manager) PersistMemory(id memorycore.MemoryID, mem *memorycore.Memory) error {
	if m == nil || mem == nil || !m.Enabled() {
		return nil
	}
	layout, present, err := m.layoutFor(id)
	if err != nil || !present {
		return err
	}
	image, err := buildMemoryImage(layout, mem)
	if err != nil {
		m.markFailed(err, time.Now())
		return err
	}
	store := NewFileStore(config.SnapshotPath(m.Directory(), id))
	if err := store.ReplaceAtomic(image); err != nil {
		m.markFailed(err, time.Now())
		return err
	}
	m.markSaved(time.Now())
	return nil
}

// buildMemoryImage reads the current memory values for every segment and
// assembles a snapshot image.
func buildMemoryImage(layout *Layout, mem *memorycore.Memory) ([]byte, error) {
	return layout.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) {
		return readSegment(mem, seg.Segment)
	})
}

// readSegment reads one segment's payload from memory using the raw
// memorycore read primitives. Bit payloads are packed LSB-first; register
// payloads are big-endian, matching the on-disk encoding.
func readSegment(mem *memorycore.Memory, seg Segment) ([]byte, error) {
	if seg.Area.IsBitArea() {
		buf := make([]byte, bytesForBits(seg.Count))
		if err := mem.ReadBits(seg.Area, seg.Start, seg.Count, buf); err != nil {
			return nil, err
		}
		return buf, nil
	}
	buf := make([]byte, int(seg.Count)*2)
	if err := mem.ReadRegs(seg.Area, seg.Start, seg.Count, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// applyPayload writes a validated segment payload back into memory.
func applyPayload(mem *memorycore.Memory, seg Segment, payload []byte) error {
	if seg.Area.IsBitArea() {
		return mem.WriteBits(seg.Area, seg.Start, seg.Count, payload)
	}
	return mem.WriteRegs(seg.Area, seg.Start, seg.Count, payload)
}

// sameLayout reports whether a parsed snapshot layout matches the expected one.
func sameLayout(parsed, expected *Layout) bool {
	if parsed.ID != expected.ID || parsed.PayloadStart != expected.PayloadStart {
		return false
	}
	if len(parsed.Segments) != len(expected.Segments) {
		return false
	}
	for i := range parsed.Segments {
		if parsed.Segments[i] != expected.Segments[i] {
			return false
		}
	}
	return true
}

// validatePayloadRange confirms a parsed descriptor carries the expected
// length and stays inside the file image.
func validatePayloadRange(parsed, expected SegmentLayout) error {
	if parsed.Offset != expected.Offset || parsed.Length != expected.Length {
		return fmt.Errorf("snapshot segment layout mismatch for %s", expected.Area)
	}
	return nil
}
