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
		Coils:          &memorycore.AreaLayout{Start: 0, Size: 16},
		DiscreteInputs: &memorycore.AreaLayout{Start: 0, Size: 16},
		HoldingRegs:    &memorycore.AreaLayout{Start: 0, Size: 8},
		InputRegs:      &memorycore.AreaLayout{Start: 0, Size: 8},
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
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0x12, 0x34}); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(memorycore.MemoryID{Port: 502, UnitID: 1}, mem); err != nil {
		t.Fatalf("RestoreMemory: %v", err)
	}
	if m.State() != StateReady {
		t.Fatalf("state = %v, want READY", m.State())
	}
	if m.Diagnostics().RestoreSource != "initial" {
		t.Fatalf("restore source = %q", m.Diagnostics().RestoreSource)
	}

	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	for _, path := range []string{config.SnapshotPath(dir, id), BackupPath(config.SnapshotPath(dir, id))} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("initial snapshot not created at %s: %v", path, err)
		}
		if _, err := ParseLayout(data); err != nil {
			t.Fatalf("initial snapshot invalid at %s: %v", path, err)
		}
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

	m2, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	dest := newMemory(t)
	if err := m2.RestoreMemory(id, dest); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if m2.Diagnostics().RestoreSource != "primary" {
		t.Fatalf("restore source = %q, want primary", m2.Diagnostics().RestoreSource)
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

func TestRestoreCorruptPrimaryFallsBackToBackup(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}

	src := newMemory(t)
	if err := src.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0xCA, 0xFE}); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(id, src); err != nil {
		t.Fatal(err)
	}

	// Corrupt the primary only.
	path := config.SnapshotPath(dir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 0xFF
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	m2, _ := New(restorePlan(dir), allocations())
	dest := newMemory(t)
	if err := m2.RestoreMemory(id, dest); err != nil {
		t.Fatalf("recovery from backup failed: %v", err)
	}
	if m2.State() != StateReady {
		t.Fatalf("state = %v, want READY after recovery", m2.State())
	}
	if m2.Diagnostics().RestoreSource != "backup" {
		t.Fatalf("restore source = %q, want backup", m2.Diagnostics().RestoreSource)
	}
	regs := make([]byte, 2)
	if err := dest.ReadRegs(memorycore.AreaHoldingRegs, 4, 1, regs); err != nil {
		t.Fatal(err)
	}
	if regs[0] != 0xCA || regs[1] != 0xFE {
		t.Fatalf("backup values not restored: %x", regs)
	}

	// Primary must have been rebuilt to a valid image from the backup.
	rebuilt, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLayout(rebuilt); err != nil {
		t.Fatalf("primary not rebuilt valid: %v", err)
	}
}

func TestRestoreCorruptBothFailsClosed(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}

	src := newMemory(t)
	if err := m.RestoreMemory(id, src); err != nil {
		t.Fatal(err)
	}
	path := config.SnapshotPath(dir, id)
	for _, p := range []string{path, BackupPath(path)} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		data[0] ^= 0xFF
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	dest := newMemory(t)
	if err := dest.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0xAB, 0xCD}); err != nil {
		t.Fatal(err)
	}
	m2, _ := New(restorePlan(dir), allocations())
	if err := m2.RestoreMemory(id, dest); err == nil {
		t.Fatal("corrupt primary+backup accepted")
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

func TestRestorePrimaryCorruptNoBackupFailsClosed(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	if err := m.RestoreMemory(id, newMemory(t)); err != nil {
		t.Fatal(err)
	}
	path := config.SnapshotPath(dir, id)
	data, _ := os.ReadFile(path)
	data[0] ^= 0xFF
	_ = os.WriteFile(path, data, 0o600)
	_ = os.Remove(BackupPath(path))

	m2, _ := New(restorePlan(dir), allocations())
	if err := m2.RestoreMemory(id, newMemory(t)); err == nil {
		t.Fatal("corrupt primary with no backup accepted")
	}
	if m2.State() != StateFailed {
		t.Fatalf("state = %v, want FAILED", m2.State())
	}
}

func TestRestoreIdentityMismatchFails(t *testing.T) {
	m, dir := newRestoreManager(t)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	if err := m.RestoreMemory(id, newMemory(t)); err != nil {
		t.Fatal(err)
	}

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
	if err := m2.RestoreMemory(memorycore.MemoryID{Port: 502, UnitID: 2}, newMemory(t)); err == nil {
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
	if data[seg.Offset] != 0xDE || data[seg.Offset+1] != 0xAD {
		t.Fatalf("persisted value wrong: %x", data[seg.Offset:seg.Offset+2])
	}
}

func TestRestoreCreatesDirectoryRelative(t *testing.T) {
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
}

func TestRestoreMigratesLegacySnapshotNames(t *testing.T) {
    m, dir := newRestoreManager(t)
    id := memorycore.MemoryID{Port: 502, UnitID: 1}
    original := newMemory(t)
    if err := original.WriteRegs(memorycore.AreaHoldingRegs, 4, 1, []byte{0x12, 0x34}); err != nil {
        t.Fatal(err)
    }
    if err := m.RestoreMemory(id, original); err != nil {
        t.Fatal(err)
    }

    current := config.SnapshotPath(dir, id)
    old := filepath.Join(dir, "mma2-502-1.bin")
    if err := os.Rename(current, old); err != nil {
        t.Fatal(err)
    }
    if err := os.Rename(BackupPath(current), BackupPath(old)); err != nil {
        t.Fatal(err)
    }

    restored := newMemory(t)
    next, err := New(restorePlan(dir), allocations())
    if err != nil {
        t.Fatal(err)
    }
    if err := next.RestoreMemory(id, restored); err != nil {
        t.Fatal(err)
    }
    data := make([]byte, 2)
    if err := restored.ReadRegs(memorycore.AreaHoldingRegs, 4, 1, data); err != nil {
        t.Fatal(err)
    }
    if data[0] != 0x12 || data[1] != 0x34 {
        t.Fatalf("legacy snapshot not restored: %x", data)
    }
    for _, p := range []string{current, BackupPath(current)} {
        if _, err := os.Stat(p); err != nil {
            t.Fatalf("migrated file %s: %v", p, err)
        }
    }
}
