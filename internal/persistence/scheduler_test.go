package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

func scheduleManager(t *testing.T) (*Manager, *Scheduler, string, memorycore.MemoryID, *memorycore.Memory) {
	t.Helper()
	dir := t.TempDir()
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem := newMemory(t)
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)
	s := NewScheduler(m)
	s.delay = 10 * time.Millisecond
	s.checkpt = time.Hour
	m.SetNotifier(s.Notify)
	return m, s, dir, id, mem
}

func TestSchedulerFlushesOnClose(t *testing.T) {
	_, s, dir, id, mem := scheduleManager(t)
	s.Start(context.Background())

	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0xCA, 0xFE}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Read the primary snapshot bytes: register 5 lives at the second register.
	data, err := os.ReadFile(config.SnapshotPath(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ParseLayout(data)
	if err != nil {
		t.Fatalf("snapshot invalid after flush: %v", err)
	}
	seg, _ := layout.segmentFor(memorycore.AreaHoldingRegs)
	off := seg.Offset + 2
	if data[off] != 0xCA || data[off+1] != 0xFE {
		t.Fatalf("flushed register wrong: %x", data[off:off+2])
	}
}

func TestSchedulerRestoresFlushedValues(t *testing.T) {
	_, s, dir, id, mem := scheduleManager(t)
	s.Start(context.Background())

	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 4, 3, []byte{0x00, 0x0A, 0x00, 0x0B, 0x00, 0x0C})
	_ = mem.WriteBits(memorycore.AreaCoils, 0, 8, []byte{0b1010_1010})
	s.Close()

	// New manager over the same directory must restore the flushed values.
	m2, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	dest := newMemory(t)
	if err := m2.RestoreMemory(id, dest); err != nil {
		t.Fatalf("restore after flush: %v", err)
	}
	regs := make([]byte, 6)
	_ = dest.ReadRegs(memorycore.AreaHoldingRegs, 4, 3, regs)
	if string(regs) != string([]byte{0x00, 0x0A, 0x00, 0x0B, 0x00, 0x0C}) {
		t.Fatalf("restored registers wrong: %x", regs)
	}
	bits := make([]byte, 1)
	_ = dest.ReadBits(memorycore.AreaCoils, 0, 8, bits)
	if bits[0] != 0b1010_1010 {
		t.Fatalf("restored bits wrong: %08b", bits[0])
	}
}

func TestSchedulerPreservesOtherBlocks(t *testing.T) {
	dir := t.TempDir()
	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 200}},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 502, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaHoldingRegs: {Start: 0, Count: 200},
		}},
	}
	m, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 200}})
	if err != nil {
		t.Fatal(err)
	}
	// Seed several registers across multiple blocks.
	for r := uint16(0); r < 200; r++ {
		if err := mem.WriteRegs(memorycore.AreaHoldingRegs, r, 1, []byte{byte(r >> 8), byte(r)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)

	s := NewScheduler(m)
	s.delay = 10 * time.Millisecond
	s.checkpt = time.Hour
	s.Start(context.Background())

	// One late write; flush must only touch its block.
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 1, 1, []byte{0x12, 0x34}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	data, _ := os.ReadFile(config.SnapshotPath(dir, id))
	layout, err := ParseLayout(data)
	if err != nil {
		t.Fatalf("snapshot invalid after targeted flush: %v", err)
	}
	seg, _ := layout.segmentFor(memorycore.AreaHoldingRegs)
	// Register 1 updated, register 0 (block 0 same block) and far registers intact.
	if got := data[seg.Offset+2 : seg.Offset+4]; got[0] != 0x12 || got[1] != 0x34 {
		t.Fatalf("target register wrong: %x", got)
	}
	if got := data[seg.Offset+100*2 : seg.Offset+100*2+2]; got[0] != byte(100>>8) || got[1] != byte(100) {
		t.Fatalf("unrelated register changed: %x", got)
	}
}

func TestSchedulerCheckpointRefreshesBackup(t *testing.T) {
	_, s, dir, id, mem := scheduleManager(t)
	// Checkpoint every flush.
	s.checkpt = 0
	s.delay = 10 * time.Millisecond
	s.Start(context.Background())

	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0xBE, 0xEF})
	s.Close()

	data, err := os.ReadFile(BackupPath(config.SnapshotPath(dir, id)))
	if err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("backup invalid: %v", err)
	}
	layout, _ := ParseLayout(data)
	seg, _ := layout.segmentFor(memorycore.AreaHoldingRegs)
	if data[seg.Offset+2] != 0xBE || data[seg.Offset+3] != 0xEF {
		t.Fatalf("backup not refreshed with flushed value: %x", data[seg.Offset+2:seg.Offset+4])
	}
}

func TestSchedulerCoalescesBurst(t *testing.T) {
	m, s, _, id, mem := scheduleManager(t)
	s.Start(context.Background())

	for r := uint16(4); r < 7; r++ {
		_ = mem.WriteRegs(memorycore.AreaHoldingRegs, r, 1, []byte{0x00, byte(r)})
	}
	// Before flush there should be coalesced dirty ranges.
	if ranges := m.DirtyRanges(id); len(ranges) != 1 {
		t.Fatalf("expected one coalesced range, got %+v", ranges)
	}
	s.Close()
	if ranges := m.DirtyRanges(id); len(ranges) != 0 {
		t.Fatalf("dirty not drained: %+v", ranges)
	}
}

func TestCoalesceDoesNotMergeAcrossSegments(t *testing.T) {
	// Two ranges that are file-adjacent but belong to different segments must
	// NOT be merged, or the flush would reinterpret one segment's bytes as
	// another's.
	got := coalesce([]DirtyRange{
		{Start: 0, Length: 4, seg: 0},
		{Start: 4, Length: 4, seg: 4}, // different segment, adjacent in file
	})
	if len(got) != 2 {
		t.Fatalf("cross-segment ranges merged: %+v", got)
	}
	// Same segment, adjacent -> merged.
	got = coalesce([]DirtyRange{
		{Start: 0, Length: 4, seg: 0},
		{Start: 4, Length: 4, seg: 0},
	})
	if len(got) != 1 || got[0].Length != 8 {
		t.Fatalf("same-segment ranges not merged: %+v", got)
	}
}

// TestSchedulerFlushDoesNotCorruptNeighboringSegment is the regression for the
// cross-segment coalescing bug: coils and holding registers are file-adjacent,
// so writing the last coil byte and the first register in one flush window must
// not corrupt the registers.
func TestSchedulerFlushDoesNotCorruptNeighboringSegment(t *testing.T) {
	dir := t.TempDir()
	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {
				{Area: memorycore.AreaCoils, Start: 0, Count: 8},       // 1 byte
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 4}, // 8 bytes, adjacent
			},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 502, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaCoils:       {Start: 0, Count: 8},
			memorycore.AreaHoldingRegs: {Start: 0, Count: 4},
		}},
	}
	m, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils:       &memorycore.AreaLayout{Start: 0, Size: 8},
		HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 4, []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)

	s := NewScheduler(m)
	s.delay = 5 * time.Millisecond
	s.checkpt = time.Hour
	s.Start(t.Context())

	// Touch the last coil byte and the first register together.
	_ = mem.WriteBits(memorycore.AreaCoils, 4, 4, []byte{0b1001})
	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0xAA, 0xBB})
	s.Close()

	data, _ := os.ReadFile(config.SnapshotPath(dir, id))
	layout, err := ParseLayout(data)
	if err != nil {
		t.Fatalf("snapshot invalid: %v", err)
	}
	regs, _ := layout.segmentFor(memorycore.AreaHoldingRegs)
	got := data[regs.Offset : regs.Offset+8]
	if got[0] != 0xAA || got[1] != 0xBB {
		t.Fatalf("first register wrong: %x", got[:2])
	}
	// Registers 1..3 must be untouched by the coil write.
	if got[2] != 0x33 || got[3] != 0x44 || got[4] != 0x55 || got[5] != 0x66 || got[6] != 0x77 || got[7] != 0x88 {
		t.Fatalf("neighboring segment corrupted: %x", got)
	}
}

func TestSchedulerFlushNonAlignedBitSegment(t *testing.T) {
	// Coils count 12: the last containing byte covers bits 8..11 plus 4 padding
	// bits. Writing the last nibble must flush without bounds errors and must
	// not disturb the first byte.
	dir := t.TempDir()
	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {{Area: memorycore.AreaCoils, Start: 0, Count: 12}},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 502, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaCoils: {Start: 0, Count: 12},
		}},
	}
	m, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{Coils: &memorycore.AreaLayout{Start: 0, Size: 12}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteBits(memorycore.AreaCoils, 0, 12, []byte{0b1111_1111, 0b0000_1111}); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)

	s := NewScheduler(m)
	s.delay = 5 * time.Millisecond
	s.checkpt = time.Hour
	s.Start(t.Context())

	// Clear bit 10 (in the second, non-aligned byte).
	_ = mem.WriteBits(memorycore.AreaCoils, 10, 1, []byte{0x00})
	s.Close()

	if m.Failed() {
		t.Fatalf("flush of non-aligned bit segment failed: %v", m.LastError())
	}
	data, _ := os.ReadFile(config.SnapshotPath(dir, id))
	layout, err := ParseLayout(data)
	if err != nil {
		t.Fatalf("snapshot invalid: %v", err)
	}
	seg, _ := layout.segmentFor(memorycore.AreaCoils)
	if data[seg.Offset] != 0xFF {
		t.Fatalf("first coil byte changed: %08b", data[seg.Offset])
	}
	// Bit 10 cleared within the second byte: 0b0000_1011.
	if data[seg.Offset+1] != 0b0000_1011 {
		t.Fatalf("second coil byte wrong: %08b", data[seg.Offset+1])
	}
}

func TestSchedulerDisabledNoOp(t *testing.T) {
	m, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewScheduler(m)
	s.Start(context.Background())
	s.Notify()
	s.Close()
}
