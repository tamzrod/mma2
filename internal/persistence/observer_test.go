package persistence

import (
	"testing"

	"mma2/internal/memorycore"
)

func observerManager(t *testing.T) (*Manager, memorycore.MemoryID) {
	t.Helper()
	dir := t.TempDir()
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	if err := m.RestoreMemory(id, newMemory(t)); err != nil {
		t.Fatal(err)
	}
	return m, id
}

func newObservedMemory(t *testing.T) *memorycore.Memory { return newMemory(t) }

func TestAttachMemoryMarksRegistersDirty(t *testing.T) {
	m, id := observerManager(t)
	mem := newMemory(t)
	m.AttachMemory(id, mem)

	// Register 5 is the second register of the holding segment (start 4).
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0xBE, 0xEF}); err != nil {
		t.Fatal(err)
	}
	ranges := m.DirtyRanges(id)
	if len(ranges) != 1 || ranges[0].Length != 2 {
		t.Fatalf("expected one 2-byte dirty range, got %+v", ranges)
	}
}

func TestAttachMemoryMarksBitsDirty(t *testing.T) {
	m, id := observerManager(t)
	mem := newMemory(t)
	m.AttachMemory(id, mem)

	if err := mem.WriteBits(memorycore.AreaCoils, 0, 12, []byte{0xFF, 0x0F}); err != nil {
		t.Fatal(err)
	}
	ranges := m.DirtyRanges(id)
	if len(ranges) != 1 {
		t.Fatalf("expected one dirty range, got %+v", ranges)
	}
	// 12 bits live within the coils segment's first ceil(12/8)=2 bytes.
	if ranges[0].Length != 2 {
		t.Fatalf("bit dirty range length = %d, want 2", ranges[0].Length)
	}
}

func TestAttachMemoryIgnoresUnpersistedArea(t *testing.T) {
	m, id := observerManager(t)
	mem := newMemory(t)
	m.AttachMemory(id, mem)

	// Input registers are allocated (size 8) but not persisted for this plan.
	if err := mem.WriteRegs(memorycore.AreaInputRegs, 0, 1, []byte{0x11, 0x22}); err != nil {
		t.Fatal(err)
	}
	if ranges := m.DirtyRanges(id); len(ranges) != 0 {
		t.Fatalf("unpersisted area marked dirty: %+v", ranges)
	}
}

func TestCoalesceMergesOverlappingAndAdjacent(t *testing.T) {
	got := coalesce([]DirtyRange{
		{Start: 10, Length: 4},
		{Start: 0, Length: 5},
		{Start: 4, Length: 8},
		{Start: 100, Length: 2},
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 ranges, got %+v", got)
	}
	if got[0].Start != 0 || got[0].Length != 14 {
		t.Fatalf("first range wrong: %+v", got[0])
	}
	if got[1].Start != 100 || got[1].Length != 2 {
		t.Fatalf("second range wrong: %+v", got[1])
	}
}

func TestDirtySnapshotClearsAndTracksGeneration(t *testing.T) {
	m, id := observerManager(t)
	mem := newMemory(t)
	m.AttachMemory(id, mem)

	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0x00, 0x01})
	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0x00, 0x02})

	ranges, gen := m.DirtySnapshot(id)
	if len(ranges) != 1 || gen != 2 {
		t.Fatalf("snapshot ranges=%+v gen=%d", ranges, gen)
	}
	if ranges[0].Start != 66 || ranges[0].Length != 4 {
		t.Fatalf("coalesced range wrong: %+v", ranges[0])
	}
	if again := m.DirtyRanges(id); len(again) != 0 {
		t.Fatalf("dirty not cleared: %+v", again)
	}
}

func TestMarkCommittedIgnoresUnknownIdentity(t *testing.T) {
	m, _ := observerManager(t)
	m.MarkCommitted(memorycore.MemoryID{Port: 999, UnitID: 1}, memorycore.AreaCoils, 0, 1)
	if ranges := m.DirtyRanges(memorycore.MemoryID{Port: 999, UnitID: 1}); len(ranges) != 0 {
		t.Fatalf("unknown identity marked dirty: %+v", ranges)
	}
}

func TestAttachMemoryDisabledIsNoOp(t *testing.T) {
	m, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemory(t)
	m.AttachMemory(memorycore.MemoryID{Port: 1, UnitID: 1}, mem)
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0x00, 0x01}); err != nil {
		t.Fatal(err)
	}
	if len(m.DirtyRanges(memorycore.MemoryID{Port: 1, UnitID: 1})) != 0 {
		t.Fatal("disabled manager marked dirty")
	}
}
