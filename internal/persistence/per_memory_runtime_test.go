package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// Exercises the same per-identity build/restore/flush path used by cmd/mma2.
// Unit 3 deliberately has no persistence; Units 1 and 2 use distinct directories.
func TestPerMemoryRestartAndFailureIsolation(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Ingress: []config.IngressGate{{
		ID: "lab", Listen: ":15040", Memory: []config.MemoryDefinition{
			{UnitID: 1, HoldingRegs: config.Area{Count: 16}, Persistence: &config.MemoryPersistenceConfig{Enabled: true, Directory: filepath.Join(root, "one")}},
			{UnitID: 2, HoldingRegs: config.Area{Count: 16}, Persistence: &config.MemoryPersistenceConfig{Enabled: true, Directory: filepath.Join(root, "two")}},
			{UnitID: 3, HoldingRegs: config.Area{Count: 16}},
		},
	}}}
	if err := config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	plans, err := config.BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 enabled memories, got %d", len(plans))
	}
	allocs := config.BuildMemoryAllocations(cfg)
	newMemory := func() *memorycore.Memory {
		m, e := memorycore.NewMemory(memorycore.MemoryLayouts{HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 16}})
		if e != nil {
			t.Fatal(e)
		}
		return m
	}
	ids := []memorycore.MemoryID{{Port: 15040, UnitID: 1}, {Port: 15040, UnitID: 2}, {Port: 15040, UnitID: 3}}
	values := []byte{0x11, 0x22, 0x33}
	for i, id := range ids {
		mem := newMemory()
		if i == 2 {
			if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0, values[i]}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		plan := plans[id]
		if err := os.MkdirAll(plan.Directory, 0700); err != nil {
			t.Fatal(err)
		}
		mgr, err := New(plan, map[memorycore.MemoryID]config.MemoryAllocation{id: allocs[id]})
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.RestoreMemory(id, mem); err != nil {
			t.Fatal(err)
		}
		mgr.AttachMemory(id, mem)
		if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0, values[i]}); err != nil {
			t.Fatal(err)
		}
		s := NewScheduler(mgr)
		s.Start(t.Context())
		s.Close()
		if mgr.Failed() {
			t.Fatal(mgr.LastError())
		}
	}
	// Restart and verify enabled memories restored independently.
	for i, id := range ids {
		mem := newMemory()
		if i < 2 {
			mgr, err := New(plans[id], map[memorycore.MemoryID]config.MemoryAllocation{id: allocs[id]})
			if err != nil {
				t.Fatal(err)
			}
			if err := mgr.RestoreMemory(id, mem); err != nil {
				t.Fatal(err)
			}
		}
		got := make([]byte, 2)
		if err := mem.ReadRegs(memorycore.AreaHoldingRegs, 0, 1, got); err != nil {
			t.Fatal(err)
		}
		want := byte(0)
		if i < 2 {
			want = values[i]
		}
		if got[1] != want {
			t.Fatalf("unit %d restored %x, want %x", id.UnitID, got[1], want)
		}
	}
	// Corrupt Unit 1's primary. Unit 2 must still recover its own latest image.
	first := plans[ids[0]]
	primary := config.SnapshotPath(first.Directory, ids[0])
	data, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err := os.WriteFile(primary, data, 0600); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids[:2] {
		mem := newMemory()
		mgr, err := New(plans[id], map[memorycore.MemoryID]config.MemoryAllocation{id: allocs[id]})
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.RestoreMemory(id, mem); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 2)
		if err := mem.ReadRegs(memorycore.AreaHoldingRegs, 0, 1, got); err != nil {
			t.Fatal(err)
		}
		want := values[i]
		if i == 0 {
			want = 0
		} // original untouched backup baseline
		if got[1] != want {
			t.Fatalf("unit %d corruption isolated incorrectly: got %x, want %x", id.UnitID, got[1], want)
		}
	}
}
