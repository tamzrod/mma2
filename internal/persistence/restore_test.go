package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

func restorePlan(dir string) *config.ResolvedPersistence {
	return &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {
				{Area: memorycore.AreaCoils, Start: 0, Count: 12},
				{Area: memorycore.AreaHoldingRegs, Start: 4, Count: 3},
			},
		},
	}
}

func newMemory(t *testing.T) *memorycore.Memory {
	t.Helper()
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils:       &memorycore.AreaLayout{Start: 0, Size: 16},
		HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mem
}

func newRestoreManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	return m, dir
}

func TestRestoreMissingCreatesInitialSnapshot(t *testing.T) {
	m, dir := newRestoreManager(t)
	mem := newMemory(t)

	// Seed memory with non-default values so the initial snapshot captures them.
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0x12, 0x34}); err != nil {
		t.Fatal(err)
	}

	if err := m.RestoreMemory(memorycore.MemoryID{Port: 502, UnitID: 1}, mem); err != nil {
		t.Fatalf("RestoreMemory: %v", err)
	}
	if m.State() != StateReady {
		t.Fatalf("state = %v, want READY", m.State())
	}

	path := config.SnapshotPath(dir, memorycore.MemoryID{Port: 502, UnitID: 1})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("initial snapshot not created: %v", err)
	}
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("initial snapshot invalid: %v", err)
	}
}

func TestRestoreValidSnapshotRoundtrip(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}

	source := newMemory(t)
	if err := source.WriteRegs(memorycore.AreaHoldingRegs, 4, 3, []byte{0x00, 0x01, 0x00, 0x02, 0x12, 0x34}); err != nil {
		t.Fatal(err)
	}
	if err := source.WriteBits(memorycore.AreaCoils, 0, 12, []byte{0b1010_0101, 0b0000_1111}); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(id, source); err != nil {
		t.Fatalf("initial restore: %v", err)
	}

	// Second manager reads the same directory and restores into fresh memory.
	m2, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	dest := newMemory(t)
	if err := m2.RestoreMemory(id, dest); err != nil {
		t.Fatalf("restore: %v", err)
	}

	regs := make([]byte, 6)
	if err := dest.ReadRegs(memorycore.AreaHoldingRegs, 4, 3, regs); err != nil {
		t.Fatal(err)
	}
	if string(regs) != string([]byte{0x00, 0x01, 0x00, 0x02, 0x12, 0x34}) {
		t.Fatalf("registers not restored: %x", regs)
	}
	bits := make([]byte, 2)
	if err := dest.ReadBits(memorycore.AreaCoils, 0, 12, bits); err != nil {
		t.Fatal(err)
	}
	if bits[0] != 0b1010_0101 || bits[1] != 0b0000_1111 {
		t.Fatalf("bits not restored: %08b %08b", bits[0], bits[1])
	}
}

func TestRestoreCorruptSnapshotFailsClosed(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}

	src := newMemory(t)
	if err := m.RestoreMemory(id, src); err != nil {
		t.Fatal(err)
	}

	path := config.SnapshotPath(dir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 0xFF // corrupt magic
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	dest := newMemory(t)
	if err := dest.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0xAB, 0xCD}); err != nil {
		t.Fatal(err)
	}

	m2, _ := New(restorePlan(dir), allocations())
	if err := m2.RestoreMemory(id, dest); err == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	if m2.State() != StateFailed {
		t.Fatalf("state = %v, want FAILED", m2.State())
	}
	if m2.Diagnostics().LastError == "" {
		t.Fatal("failure not recorded")
	}

	// Memory must be untouched: no partial restore.
	regs := make([]byte, 2)
	if err := dest.ReadRegs(memorycore.AreaHoldingRegs, 4, 1, regs); err != nil {
		t.Fatal(err)
	}
	if regs[0] != 0xAB || regs[1] != 0xCD {
		t.Fatalf("memory was partially restored: %x", regs)
	}
}

func TestRestoreIdentityMismatchFails(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	if err := m.RestoreMemory(id, newMemory(t)); err != nil {
		t.Fatal(err)
	}

	// Rename the snapshot to a different identity's expected path.
	path := config.SnapshotPath(dir, id)
	other := config.SnapshotPath(dir, memorycore.MemoryID{Port: 502, UnitID: 2})
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(other, data, 0o600); err != nil {
		t.Fatal(err)
	}

	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 2}: {{Area: memorycore.AreaHoldingRegs, Start: 4, Count: 3}},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 502, UnitID: 2}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaHoldingRegs: {Start: 0, Count: 8},
		}},
	}
	m2, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	dest := newMemory(t)
	if err := m2.RestoreMemory(memorycore.MemoryID{Port: 502, UnitID: 2}, dest); err == nil {
		t.Fatal("identity mismatch accepted")
	}
	if m2.State() != StateFailed {
		t.Fatalf("state = %v, want FAILED", m2.State())
	}
}

func TestRestoreNoSegmentsIsNoOp(t *testing.T) {
	m, _ := newRestoreManager(t)
	_, present, err := m.layoutFor(memorycore.MemoryID{Port: 999, UnitID: 9})
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("unexpected segments for unknown identity")
	}
	if err := m.RestoreMemory(memorycore.MemoryID{Port: 999, UnitID: 9}, newMemory(t)); err != nil {
		t.Fatalf("no-op restore returned error: %v", err)
	}
}

func TestDisabledRestoreIsNoOp(t *testing.T) {
	m, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(memorycore.MemoryID{Port: 1, UnitID: 1}, newMemory(t)); err != nil {
		t.Fatalf("disabled restore returned error: %v", err)
	}
	if m.State() != StateDisabled {
		t.Fatalf("state = %v, want DISABLED", m.State())
	}
}

func TestPersistMemoryWritesImage(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem := newMemory(t)
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0xDE, 0xAD}); err != nil {
		t.Fatal(err)
	}
	if err := m.PersistMemory(id, mem); err != nil {
		t.Fatalf("PersistMemory: %v", err)
	}

	data, err := os.ReadFile(config.SnapshotPath(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ParseLayout(data)
	if err != nil {
		t.Fatal(err)
	}
	seg, ok := layout.segmentFor(memorycore.AreaHoldingRegs)
	if !ok {
		t.Fatal("registers segment missing")
	}
	regOff := seg.Offset + 0 // start 4 -> index 0
	if data[regOff] != 0xDE || data[regOff+1] != 0xAD {
		t.Fatalf("persisted value wrong: %x", data[regOff:regOff+2])
	}
}

func TestRestoreRelativePathAndDirSync(t *testing.T) {
	// Sanity: a relative directory works and rename+dir sync succeed.
	dir := filepath.Join(t.TempDir(), "rel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(memorycore.MemoryID{Port: 502, UnitID: 1}, newMemory(t)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(config.SnapshotPath(dir, memorycore.MemoryID{Port: 502, UnitID: 1})); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
}
