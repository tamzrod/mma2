package persistence

import (
	"os"
	"sync"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

func multiPlan(dir string) (*config.ResolvedPersistence, map[memorycore.MemoryID]config.MemoryAllocation) {
	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 4},
			},
			{Port: 503, UnitID: 1}: {
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 4},
			},
			{Port: 502, UnitID: 2}: {
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 4},
			},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{}
	for _, id := range []memorycore.MemoryID{{Port: 502, UnitID: 1}, {Port: 503, UnitID: 1}, {Port: 502, UnitID: 2}} {
		allocs[id] = config.MemoryAllocation{Areas: map[memorycore.Area]config.Area{
			memorycore.AreaHoldingRegs: {Start: 0, Count: 4},
		}}
	}
	return plan, allocs
}

// TestSnapshotsIsolatedPerIdentity verifies distinct identities get distinct
// files and their values cannot cross.
func TestSnapshotsIsolatedPerIdentity(t *testing.T) {
	dir := t.TempDir()
	plan, allocs := multiPlan(dir)
	m, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}

	want := map[memorycore.MemoryID][]byte{
		{Port: 502, UnitID: 1}: {0x11, 0x11},
		{Port: 503, UnitID: 1}: {0x22, 0x22},
		{Port: 502, UnitID: 2}: {0x33, 0x33},
	}
	for id, value := range want {
		mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
		if err != nil {
			t.Fatal(err)
		}
		if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, value); err != nil {
			t.Fatal(err)
		}
		if err := m.RestoreMemory(id, mem); err != nil {
			t.Fatal(err)
		}
	}

	// Distinct files exist.
	seen := map[string]bool{}
	for id := range want {
		path := config.SnapshotPath(dir, id)
		if seen[path] {
			t.Fatalf("identity path collision at %s", path)
		}
		seen[path] = true
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("snapshot missing for %v: %v", id, err)
		}
	}

	// Restore each identity into fresh memory and confirm no crossing.
	for id, value := range want {
		mem, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
		if err := m.RestoreMemory(id, mem); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 2)
		_ = mem.ReadRegs(memorycore.AreaHoldingRegs, 0, 1, got)
		if got[0] != value[0] || got[1] != value[1] {
			t.Fatalf("identity %v restored %x, want %x", id, got, value)
		}
	}
}

// TestStateSealingUnaffected verifies persistence does not alter State Sealing
// metadata or behavior.
func TestStateSealingUnaffected(t *testing.T) {
	dir := t.TempDir()
	plan, allocs := multiPlan(dir)
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
	sealDef := memorycore.StateSealingDef{Area: memorycore.AreaCoils, Address: 0, ExceptionCode: 0x0B}
	mem.SetStateSealing(sealDef)

	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)

	if got := mem.StateSealing(); got == nil || *got != sealDef {
		t.Fatalf("state sealing metadata changed: %+v", got)
	}
	// Persistence never exposes its state as a coil or a memory address: the
	// sealing flag lives at coil 0 and is untouched by restore.
	bits := make([]byte, 1)
	_ = mem.ReadBits(memorycore.AreaCoils, 0, 1, bits)
	if bits[0] != 0 {
		t.Fatalf("coil 0 unexpectedly set: %08b", bits[0])
	}
}

// TestCrossMemoryConcurrency runs concurrent writes across identities and
// flushes them; no identity may receive another's bytes.
func TestCrossMemoryConcurrency(t *testing.T) {
	dir := t.TempDir()
	plan, allocs := multiPlan(dir)
	m, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}

	ids := []memorycore.MemoryID{{Port: 502, UnitID: 1}, {Port: 503, UnitID: 1}, {Port: 502, UnitID: 2}}
	mems := map[memorycore.MemoryID]*memorycore.Memory{}
	values := map[memorycore.MemoryID][]byte{}
	for i, id := range ids {
		mem, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
		mems[id] = mem
		values[id] = []byte{byte(0xA0 + i), byte(0x0B + i)}
		if err := m.RestoreMemory(id, mem); err != nil {
			t.Fatal(err)
		}
		m.AttachMemory(id, mem)
	}

	s := NewScheduler(m)
	s.delay = 5 * time.Millisecond
	s.checkpt = time.Hour
	s.Start(t.Context())

	var wg sync.WaitGroup
	for _, id := range ids {
		for w := 0; w < 20; w++ {
			wg.Add(1)
			go func(id memorycore.MemoryID) {
				defer wg.Done()
				_ = mems[id].WriteRegs(memorycore.AreaHoldingRegs, 7%4, 1, values[id])
			}(id)
		}
	}
	wg.Wait()
	s.Close()

	// Each identity's snapshot must contain only its own value.
	for _, id := range ids {
		m2, _ := New(plan, allocs)
		dest, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
		if err := m2.RestoreMemory(id, dest); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 2)
		_ = dest.ReadRegs(memorycore.AreaHoldingRegs, 7%4, 1, got)
		if got[0] != values[id][0] || got[1] != values[id][1] {
			t.Fatalf("identity %v got %x want %x", id, got, values[id])
		}
	}
}

// TestNoExternalServiceRequired verifies a restore works with no listeners, no
// RBE, and no network.
func TestNoExternalServiceRequired(t *testing.T) {
	dir := t.TempDir()
	plan, allocs := multiPlan(dir)
	m, _ := New(plan, allocs)
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0xDE, 0xAD})
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}

	// A fresh manager over the same files restores without any transport.
	m2, _ := New(plan, allocs)
	dest, _ := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4}})
	if err := m2.RestoreMemory(id, dest); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 2)
	_ = dest.ReadRegs(memorycore.AreaHoldingRegs, 0, 1, got)
	if got[0] != 0xDE || got[1] != 0xAD {
		t.Fatalf("restore without external service failed: %x", got)
	}
}
