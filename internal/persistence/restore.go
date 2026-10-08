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

// RestoreMemory restores one identity's persisted state before it is exposed to
// any protocol listener.
//
//   - Missing primary and backup: memory keeps its normal initial values; an
//     initial snapshot is written as both primary and backup.
//   - Valid primary: its payload is applied.
//   - Invalid primary with a valid backup: the backup is applied, reported as
//     degraded recovery, and used to rebuild the primary.
//   - Invalid primary and invalid (or missing) backup: fails closed; no partial
//     state is applied and the manager transitions to FAILED. Fatal to startup.
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
	store.SetLayout(layout)

	primaryExists, err := store.PrimaryExists()
	if err != nil {
		return m.fail(id, fmt.Errorf("stat primary: %w", err))
	}
	backupExists, err := store.BackupExists()
	if err != nil {
		return m.fail(id, fmt.Errorf("stat backup: %w", err))
	}

	// No usable snapshot at all: initialize and establish primary + backup.
	if !primaryExists && !backupExists {
		image, err := buildMemoryImage(layout, mem)
		if err != nil {
			return m.fail(id, err)
		}
		if err := store.ReplaceBoth(image); err != nil {
			return m.fail(id, fmt.Errorf("create initial snapshot: %w", err))
		}
		m.markRestoredFrom("initial", time.Now())
		return nil
	}

	// Prefer the primary; fall back to the backup only if the primary is invalid.
	if primaryExists {
		data, err := store.ReadPrimary()
		if err != nil {
			return m.fail(id, fmt.Errorf("read primary: %w", err))
		}
		if parsed, err := ParseLayout(data); err == nil && parsed.ID == id && sameLayout(parsed, layout) {
			if err := applyLayout(data, parsed, layout, mem); err != nil {
				return m.fail(id, err)
			}
			m.markRestoredFrom("primary", time.Now())
			return nil
		}
	}

	// Primary missing or invalid: try the backup.
	if backupExists {
		data, err := store.ReadBackup()
		if err != nil {
			return m.fail(id, fmt.Errorf("read backup: %w", err))
		}
		parsed, err := ParseLayout(data)
		if err == nil && parsed.ID == id && sameLayout(parsed, layout) {
			if err := applyLayout(data, parsed, layout, mem); err != nil {
				return m.fail(id, err)
			}
			if err := store.ReplaceAtomic(data); err != nil {
				return m.fail(id, fmt.Errorf("rebuild primary from backup: %w", err))
			}
			m.markRestoredFrom("backup", time.Now())
			return nil
		}
	}

	return m.fail(id, fmt.Errorf("no valid snapshot (primary or backup)"))
}

// fail records an identity restore failure and returns a wrapped error.
func (m *Manager) fail(id memorycore.MemoryID, err error) error {
	wrapped := fmt.Errorf("persistence: identity (port=%d unit=%d): %w", id.Port, id.UnitID, err)
	m.markFailed(wrapped, time.Now())
	return wrapped
}

// applyLayout validates every segment payload before applying anything, so a
// failure never leaves memory partly restored.
func applyLayout(image []byte, parsed, layout *Layout, mem *memorycore.Memory) error {
	for i, seg := range layout.Segments {
		if parsed.Segments[i].Offset != seg.Offset || parsed.Segments[i].Length != seg.Length {
			return fmt.Errorf("snapshot segment layout mismatch for %s", seg.Area)
		}
	}
	for i, seg := range layout.Segments {
		payload := image[parsed.Segments[i].Offset : parsed.Segments[i].Offset+parsed.Segments[i].Length]
		if err := applyPayload(mem, seg.Segment, payload); err != nil {
			return fmt.Errorf("apply %s: %w", seg.Area, err)
		}
	}
	return nil
}

// PersistMemory writes the current memory values as a complete snapshot image
// using atomic replacement of both primary and backup. Used for initial
// creation and layout rebuilds; the dirty scheduler (P08) uses targeted writes
// and refreshes the backup on a checkpoint schedule.
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
		return m.fail(id, err)
	}
	store := NewFileStore(config.SnapshotPath(m.Directory(), id))
	store.SetLayout(layout)
	if err := store.ReplaceBoth(image); err != nil {
		return m.fail(id, err)
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
	if parsed.ID != expected.ID ||
		parsed.PayloadStart != expected.PayloadStart ||
		parsed.BlockSize != expected.BlockSize ||
		parsed.BlockCRCOffset != expected.BlockCRCOffset ||
		parsed.BlockCount != expected.BlockCount ||
		parsed.TotalSize != expected.TotalSize {
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
