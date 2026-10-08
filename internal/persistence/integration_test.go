package persistence

import (
	"os"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// TestIntegrationRestartRoundtrip exercises the documented end-to-end flow:
// start -> write -> flush -> stop -> restart -> read the same values, with
// neighboring bits/words preserved and no external assistance.
func TestIntegrationRestartRoundtrip(t *testing.T) {
	dir := t.TempDir()
	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 15020, UnitID: 1}: {
				{Area: memorycore.AreaCoils, Start: 0, Count: 16},
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 16},
			},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 15020, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaCoils:       {Start: 0, Count: 16},
			memorycore.AreaHoldingRegs: {Start: 0, Count: 16},
		}},
	}
	id := memorycore.MemoryID{Port: 15020, UnitID: 1}

	newMem := func() *memorycore.Memory {
		mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
			Coils:       &memorycore.AreaLayout{Start: 0, Size: 16},
			HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 16},
		})
		if err != nil {
			t.Fatal(err)
		}
		return mem
	}

	// First "process" run.
	m1, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	mem1 := newMem()
	// Establish an initial snapshot with baseline values.
	_ = mem1.WriteRegs(memorycore.AreaHoldingRegs, 0, 16, seqRegs16(0x0100))
	_ = mem1.WriteBits(memorycore.AreaCoils, 0, 16, []byte{0xFF, 0xFF})
	if err := m1.RestoreMemory(id, mem1); err != nil {
		t.Fatal(err)
	}
	m1.AttachMemory(id, mem1)

	s1 := NewScheduler(m1)
	s1.delay = 5 * time.Millisecond
	s1.checkpt = time.Hour
	s1.Start(t.Context())

	// A few targeted changes that must survive.
	_ = mem1.WriteRegs(memorycore.AreaHoldingRegs, 3, 1, []byte{0xBE, 0xEF})
	_ = mem1.WriteBits(memorycore.AreaCoils, 1, 1, []byte{0x00}) // clear coil 1
	s1.Close()                                                   // orderly shutdown flushes

	// Second "process" run over the same directory.
	m2, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	mem2 := newMem()
	if err := m2.RestoreMemory(id, mem2); err != nil {
		t.Fatalf("restart restore: %v", err)
	}
	if m2.Diagnostics().RestoreSource != "primary" {
		t.Fatalf("restore source = %q", m2.Diagnostics().RestoreSource)
	}

	regs := make([]byte, 32)
	_ = mem2.ReadRegs(memorycore.AreaHoldingRegs, 0, 16, regs)
	if regs[6] != 0xBE || regs[7] != 0xEF {
		t.Fatalf("changed register not restored: %x", regs[6:8])
	}
	// Neighboring registers unchanged.
	if regs[4] != 0x01 || regs[5] != 0x03 || regs[8] != 0x01 || regs[9] != 0x05 {
		t.Fatalf("neighboring registers changed: %x", regs[4:10])
	}

	bits := make([]byte, 2)
	_ = mem2.ReadBits(memorycore.AreaCoils, 0, 16, bits)
	// Coil 1 cleared, all other bits still set.
	if bits[0] != 0b1111_1101 || bits[1] != 0xFF {
		t.Fatalf("coil state not restored: %08b %08b", bits[0], bits[1])
	}
}

// TestIntegrationCorruptPrimaryRecoversFromBackup covers the documented
// recovery path: corrupt primary falls back to a valid backup.
func TestIntegrationCorruptPrimaryRecoversFromBackup(t *testing.T) {
	dir := t.TempDir()
	plan, allocs := multiPlan(dir)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}

	m1, _ := New(plan, allocs)
	mem1, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
	_ = mem1.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0x55, 0x66})
	if err := m1.RestoreMemory(id, mem1); err != nil {
		t.Fatal(err)
	}

	primary := config.SnapshotPath(dir, id)
	data, _ := os.ReadFile(primary)
	data[len(data)-1] ^= 0x01 // corrupt a payload byte -> block CRC mismatch
	_ = os.WriteFile(primary, data, 0o600)

	m2, _ := New(plan, allocs)
	dest, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
	if err := m2.RestoreMemory(id, dest); err != nil {
		t.Fatalf("expected recovery from backup: %v", err)
	}
	if m2.Diagnostics().RestoreSource != "backup" {
		t.Fatalf("restore source = %q, want backup", m2.Diagnostics().RestoreSource)
	}
	got := make([]byte, 2)
	_ = dest.ReadRegs(memorycore.AreaHoldingRegs, 0, 1, got)
	if got[0] != 0x55 || got[1] != 0x66 {
		t.Fatalf("backup value not restored: %x", got)
	}
}

// TestIntegrationDisabledPreservesOldBehavior verifies a config without
// persistence behaves exactly as before: no files, no dirs created.
func TestIntegrationDisabledPreservesOldBehavior(t *testing.T) {
	m, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Enabled() {
		t.Fatal("nil plan enabled persistence")
	}
	mem, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0x01, 0x02}); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(memorycore.MemoryID{Port: 502, UnitID: 1}, mem); err != nil {
		t.Fatal(err)
	}
	s := NewScheduler(m)
	s.Start(t.Context())
	s.Notify()
	s.Close()
	if m.State() != StateDisabled {
		t.Fatalf("state = %v", m.State())
	}
}

// TestIntegrationLayoutChangeFailsClosed verifies changed ranges/layout reject
// an existing snapshot rather than misapplying data.
func TestIntegrationLayoutChangeFailsClosed(t *testing.T) {
	dir := t.TempDir()
	plan, allocs := multiPlan(dir)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	m1, _ := New(plan, allocs)
	mem1, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
	if err := m1.RestoreMemory(id, mem1); err != nil {
		t.Fatal(err)
	}

	// Second manager with a different range for the same identity.
	changedPlan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 8}},
		},
	}
	changedAllocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 502, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaHoldingRegs: {Start: 0, Count: 8},
		}},
	}
	m2, _ := New(changedPlan, changedAllocs)
	mem2, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 8}})
	if err := m2.RestoreMemory(id, mem2); err == nil {
		t.Fatal("changed layout accepted an existing snapshot")
	}
	if !m2.Failed() {
		t.Fatalf("state = %v, want FAILED", m2.State())
	}
}

func seqRegs16(base uint16) []byte {
	out := make([]byte, 32)
	for i := 0; i < 16; i++ {
		out[i*2] = byte((base + uint16(i) + 1) >> 8)
		out[i*2+1] = byte((base + uint16(i) + 1))
	}
	return out
}
