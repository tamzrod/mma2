package config

import (
	"path/filepath"
	"strings"
	"testing"

	"mma2/internal/memorycore"
)

func persistenceConfig() *Config {
	return &Config{Ingress: []IngressGate{
		{ID: "main", Listen: "127.0.0.1:15030", Memory: []MemoryDefinition{
			{UnitID: 1, HoldingRegs: Area{Start: 0, Count: 32}, Coils: Area{Start: 0, Count: 16},
				Persistence: &MemoryPersistenceConfig{Enabled: true, Directory: "test/snapshots/unit1"}},
			{UnitID: 2, HoldingRegs: Area{Start: 0, Count: 32}},
		}},
	}}
}

func TestPerMemoryPersistenceDefaultsDisabled(t *testing.T) {
	cfg := persistenceConfig()
	cfg.Ingress[0].Memory[0].Persistence = nil
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 {
		t.Fatalf("disabled memory got plans: %v", plans)
	}
}

func TestPerMemoryPersistenceIsolation(t *testing.T) {
	cfg := persistenceConfig()
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 15030, UnitID: 1}
	if len(plans) != 1 || plans[id] == nil || !plans[id].RangesEmpty {
		t.Fatalf("expected only Unit ID 1 with full-area persistence: %+v", plans)
	}
	if _, ok := plans[memorycore.MemoryID{Port: 15030, UnitID: 2}]; ok {
		t.Fatal("disabled Unit ID 2 unexpectedly persisted")
	}
}

func TestRootPersistenceRejected(t *testing.T) {
	cfg := persistenceConfig()
	cfg.Persistence = &PersistenceConfig{}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "root-level") {
		t.Fatalf("root persistence must be rejected, got %v", err)
	}
}

func TestPerMemoryPersistenceDefaultsToYAMLDirectory(t *testing.T) {
	cfg := persistenceConfig()
	cfg.Ingress[0].Memory[0].Persistence.Directory = ""
	cfg.configDir = t.TempDir()
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 15030, UnitID: 1}
	if plans[id].Directory != cfg.configDir {
		t.Fatalf("default directory = %q; want %q", plans[id].Directory, cfg.configDir)
	}
}

func TestPerMemoryPersistenceConfiguredRanges(t *testing.T) {
	cfg := persistenceConfig()
	cfg.Ingress[0].Memory[0].Persistence.Ranges = &MemoryPersistenceRanges{
		HoldingRegs: []PersistenceArea{{Start: 0, Count: 10}, {Start: 20, Count: 10}},
	}
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 15030, UnitID: 1}
	if plans[id].RangesEmpty || len(plans[id].Ranges[id]) != 2 {
		t.Fatalf("incorrect ranges: %+v", plans[id])
	}
}

func TestPerMemoryPersistenceRejectsInvalidRanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ranges MemoryPersistenceRanges
	}{
		{"zero", MemoryPersistenceRanges{HoldingRegs: []PersistenceArea{{Start: 0, Count: 0}}}},
		{"gap", MemoryPersistenceRanges{HoldingRegs: []PersistenceArea{{Start: 40, Count: 1}}}},
		{"overlap", MemoryPersistenceRanges{HoldingRegs: []PersistenceArea{{Start: 0, Count: 10}, {Start: 5, Count: 10}}}},
		{"unallocated", MemoryPersistenceRanges{DiscreteInputs: []PersistenceArea{{Start: 0, Count: 1}}}},
		{"empty", MemoryPersistenceRanges{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := persistenceConfig()
			cfg.Ingress[0].Memory[0].Persistence.Ranges = &tc.ranges
			if err := Validate(cfg); err == nil {
				t.Fatal("invalid ranges accepted")
			}
		})
	}
}

func TestPerMemoryPersistenceAllowsSharedDirectory(t *testing.T) {
	cfg := persistenceConfig()
	cfg.Ingress[0].Memory[1].Persistence = &MemoryPersistenceConfig{Enabled: true, Directory: "test/snapshots/unit1"}
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected two plans, got %d", len(plans))
	}
}

func TestPerMemoryPersistenceYAMLParsing(t *testing.T) {
	raw := `listeners:
  - id: lab
    listen: ":15030"
    memory:
      - unit_id: 1
        holding_registers: {start: 0, count: 32}
        persistence:
          enabled: true
          directory: test/persistence_manual/snapshots
          ranges:
            holding_registers:
              - start: 0
                count: 10
              - start: 20
                count: 10
`
	cfg, err := Load(writeTemp(t, []byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 15030, UnitID: 1}
	if len(plans[id].Ranges[id]) != 2 {
		t.Fatalf("YAML ranges not loaded: %+v", plans[id])
	}
}

func TestSnapshotFileNamesAreDeterministicAndDistinct(t *testing.T) {
	a := SnapshotFileName(memorycore.MemoryID{Port: 502, UnitID: 1})
	b := SnapshotFileName(memorycore.MemoryID{Port: 502, UnitID: 1})
	c := SnapshotFileName(memorycore.MemoryID{Port: 503, UnitID: 1})
	if a != b || a == c || a != "502-1.bin" {
		t.Fatalf("snapshot identity collision: %q %q %q", a, b, c)
	}
}

func TestLoadedYAMLDefaultsPersistenceToConfigFolder(t *testing.T) {
	raw := []byte(`listeners:
  - id: test
    listen: ":15030"
    memory:
      - unit_id: 1
        holding_registers: {start: 0, count: 10}
        persistence:
          enabled: true
      - unit_id: 2
        holding_registers: {start: 0, count: 10}
        persistence:
          enabled: true
`)
	filename := writeTemp(t, raw)
	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	plans, err := BuildPerMemoryPersistencePlans(cfg)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Dir(filename)
	for _, unit := range []uint16{1, 2} {
		id := memorycore.MemoryID{Port: 15030, UnitID: unit}
		if plans[id] == nil || plans[id].Directory != folder {
			t.Fatalf("unit %d directory = %+v, want %q", unit, plans[id], folder)
		}
	}
}
