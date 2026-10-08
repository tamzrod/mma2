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
