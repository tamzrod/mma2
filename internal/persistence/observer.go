package persistence

import (
	"sort"

	"mma2/internal/memorycore"
)

// DirtyRange is a contiguous byte range within a snapshot file that has changed
// and is not yet flushed.
type DirtyRange struct {
	Start  uint32
	Length uint32
}

// memoryObserver is the per-identity bridge from memorycore committed writes to
// persistence dirty marking. It performs no disk IO and only records changed
// byte ranges under the manager mutex.
type memoryObserver struct {
	mgr *Manager
	id  memorycore.MemoryID
}

func (o memoryObserver) OnCommittedWrite(area memorycore.Area, address, count uint16) {
	o.mgr.MarkCommitted(o.id, area, address, count)
}

// AttachMemory installs the committed-write observer on one memory so that every
// successful committed write marks persistence dirty. It is a no-op when
// persistence is disabled or the identity has no persisted segments.
//
// Must be called during startup, before concurrent writers run.
func (m *Manager) AttachMemory(id memorycore.MemoryID, mem *memorycore.Memory) {
	if m == nil || mem == nil || !m.Enabled() {
		return
	}
	if _, ok := m.Segments(id); !ok {
		return
	}
	mem.SetCommittedWriteObserver(memoryObserver{mgr: m, id: id})
	m.mu.Lock()
	if m.memories == nil {
		m.memories = make(map[memorycore.MemoryID]*memorycore.Memory)
	}
	m.memories[id] = mem
	m.mu.Unlock()
}

// MarkCommitted records the file byte range(s) made dirty by one committed
// write. Ranges outside the identity's persisted segments are ignored, so an
// unpersisted memory or area never marks persistence dirty.
func (m *Manager) MarkCommitted(id memorycore.MemoryID, area memorycore.Area, address, count uint16) {
	if m == nil || count == 0 {
		return
	}
	layout, ok := m.LayoutFor(id)
	if !ok {
		return
	}

	ranges := committedDirtyRanges(layout, area, address, count)
	if len(ranges) == 0 {
		return
	}

	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[memorycore.MemoryID][]DirtyRange)
	}
	m.dirty[id] = coalesce(append(m.dirty[id], ranges...))
	if m.dirtyGen == nil {
		m.dirtyGen = make(map[memorycore.MemoryID]uint64)
	}
	m.dirtyGen[id]++
	notify := m.notifier
	m.mu.Unlock()

	if notify != nil {
		notify()
	}
}

// SetNotifier installs a callback invoked (without holding the manager lock)
// whenever an identity is marked dirty.
func (m *Manager) SetNotifier(notify func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.notifier = notify
	m.mu.Unlock()
}

// committedDirtyRanges computes the exact file byte ranges touched by a write,
// intersected with the persisted segments.
func committedDirtyRanges(layout *Layout, area memorycore.Area, address, count uint16) []DirtyRange {
	var out []DirtyRange
	end := uint32(address) + uint32(count)
	for _, seg := range layout.Segments {
		if seg.Area != area {
			continue
		}
		segEnd := uint32(seg.Start) + uint32(seg.Count)
		lo := max32(uint32(address), uint32(seg.Start))
		hi := min32(end, segEnd)
		if lo >= hi {
			continue
		}

		start, length := byteSpanWithinSegment(seg, lo, hi)
		out = append(out, DirtyRange{Start: start, Length: length})
	}
	return out
}

// byteSpanWithinSegment converts an address span [lo, hi) inside a segment to a
// file byte range.
func byteSpanWithinSegment(seg SegmentLayout, lo, hi uint32) (start, length uint32) {
	if seg.Area.IsBitArea() {
		firstByte := (lo - uint32(seg.Start)) / 8
		lastByte := (hi - 1 - uint32(seg.Start)) / 8
		return seg.Offset + firstByte, lastByte - firstByte + 1
	}
	firstByte := (lo - uint32(seg.Start)) * 2
	lastByte := (hi - uint32(seg.Start)) * 2
	return seg.Offset + firstByte, lastByte - firstByte
}

func max32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// coalesce sorts and merges overlapping or adjacent ranges.
func coalesce(ranges []DirtyRange) []DirtyRange {
	if len(ranges) == 0 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start < ranges[j].Start })
	out := ranges[:1]
	for _, r := range ranges[1:] {
		last := &out[len(out)-1]
		if r.Start <= last.Start+last.Length {
			end := max32(last.Start+last.Length, r.Start+r.Length)
			last.Length = end - last.Start
			continue
		}
		out = append(out, r)
	}
	return out
}

// remarkDirty re-adds ranges after a failed flush so the unpersisted data stays
// visible to diagnostics and a future recovery attempt.
func (m *Manager) remarkDirty(id memorycore.MemoryID, ranges []DirtyRange) {
	if m == nil || len(ranges) == 0 {
		return
	}
	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[memorycore.MemoryID][]DirtyRange)
	}
	m.dirty[id] = coalesce(append(m.dirty[id], ranges...))
	m.mu.Unlock()
}

// DirtySnapshot returns and clears the current dirty ranges for an identity.
// The generation counter increments on every marked write; a flush may use it
// to detect writes that arrived during a snapshot/copy.
func (m *Manager) DirtySnapshot(id memorycore.MemoryID) ([]DirtyRange, uint64) {
	if m == nil {
		return nil, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ranges := m.dirty[id]
	gen := m.dirtyGen[id]
	delete(m.dirty, id)
	return ranges, gen
}

// DirtyRanges returns a copy of the current dirty ranges for an identity
// without clearing them. Intended for diagnostics and tests.
func (m *Manager) DirtyRanges(id memorycore.MemoryID) []DirtyRange {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.dirty[id]
	out := make([]DirtyRange, len(src))
	copy(out, src)
	return out
}
