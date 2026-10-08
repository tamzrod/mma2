package config

import (
	"strings"
	"testing"

	"mma2/internal/memorycore"
)

func validPersistenceConfig() *Config {
	return &Config{
		Persistence: &PersistenceConfig{Enabled: true, Directory: "/var/lib/mma2"},
		Ingress: []IngressGate{{ID: "lab", Listen: "127.0.0.1:502", Memory: []MemoryDefinition{{
			UnitID:      1,
			Coils:       Area{Start: 0, Count: 16},
			HoldingRegs: Area{Start: 0, Count: 32},
			InputRegs:   Area{Start: 0, Count: 32},
		}}}},
	}
}

func TestPersistenceDisabledByDefault(t *testing.T) {
	cfg := validPersistenceConfig()
	cfg.Persistence = nil
	if err := Validate(cfg); err != nil {
		t.Fatalf("nil persistence block rejected: %v", err)
	}
	plan, err := BuildPersistencePlan(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan != nil {
		t.Fatalf("disabled persistence produced a plan: %+v", plan)
	}
}

func TestPersistenceDisabledExplicit(t *testing.T) {
	cfg := validPersistenceConfig()
	cfg.Persistence.Enabled = false
	cfg.Persistence.Directory = ""
	// Ranges are intentionally left invalid to prove validation is skipped.
	cfg.Persistence.Ranges = []PersistenceRange{{Port: 1, UnitID: 1}}
	if err := Validate(cfg); err != nil {
		t.Fatalf("disabled persistence validated contents: %v", err)
	}
}

func TestPersistenceEnabledRequiresDirectory(t *testing.T) {
	cfg := validPersistenceConfig()
	cfg.Persistence.Directory = "   "
	if err := Validate(cfg); err == nil {
		t.Fatal("enabled persistence without directory accepted")
	}
}

func TestPersistenceEnabledNoRangesAccepted(t *testing.T) {
	cfg := validPersistenceConfig()
	plan, err := BuildPersistencePlan(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan == nil || !plan.RangesEmpty {
		t.Fatalf("expected ranges-empty plan, got %+v", plan)
	}
	if len(plan.Ranges) != 0 {
		t.Fatalf("expected no explicit ranges, got %+v", plan.Ranges)
	}
}

func TestPersistenceValuesSurviveLoad(t *testing.T) {
	yaml := `persistence:
  enabled: true
  directory: /var/lib/mma2
  ranges:
    - port: 502
      unit_id: 1
      holding_registers: {start: 4, count: 8}
listeners:
  - id: lab
    listen: ":502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 16
`
	path := writeTemp(t, []byte(yaml))
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.Persistence == nil || !cfg.Persistence.Enabled || cfg.Persistence.Directory != "/var/lib/mma2" {
		t.Fatalf("persistence not parsed: %+v", cfg.Persistence)
	}
	if len(cfg.Persistence.Ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(cfg.Persistence.Ranges))
	}
	s := cfg.Persistence.Ranges[0].HoldingRegs
	if s == nil || s.Start != 4 || s.Count != 8 {
		t.Fatalf("holding_registers range not parsed: %+v", s)
	}
}

func TestPersistenceRangesRejectInvalid(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"port zero", func(c *Config) { c.Persistence.Ranges[0].Port = 0 }},
		{"unit over 255", func(c *Config) { c.Persistence.Ranges[0].UnitID = 256 }},
		{"no area selection", func(c *Config) { c.Persistence.Ranges[0].HoldingRegs = nil }},
		{"unknown identity", func(c *Config) { c.Persistence.Ranges[0].UnitID = 9 }},
		{"count zero", func(c *Config) { c.Persistence.Ranges[0].HoldingRegs.Count = 0 }},
		{"start below allocation", func(c *Config) {
			c.Persistence.Ranges[0].HoldingRegs = &PersistenceArea{Start: 0, Count: 1}
			c.Ingress[0].Memory[0].HoldingRegs = Area{Start: 4, Count: 8}
		}},
		{"end beyond allocation", func(c *Config) {
			c.Persistence.Ranges[0].HoldingRegs = &PersistenceArea{Start: 30, Count: 8}
		}},
		{"unallocated area", func(c *Config) {
			c.Persistence.Ranges[0].DiscreteInputs = &PersistenceArea{Start: 0, Count: 4}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validPersistenceConfig()
			cfg.Persistence.Ranges = []PersistenceRange{{Port: 502, UnitID: 1, HoldingRegs: &PersistenceArea{Start: 0, Count: 32}}}
			tc.mutate(cfg)
			if err := Validate(cfg); err == nil {
				t.Fatal("invalid persistence range accepted")
			}
		})
	}
}

func TestPersistenceRejectsDuplicateIdentity(t *testing.T) {
	cfg := validPersistenceConfig()
	cfg.Persistence.Ranges = []PersistenceRange{
		{Port: 502, UnitID: 1, Coils: &PersistenceArea{Start: 0, Count: 8}},
		{Port: 502, UnitID: 1, HoldingRegs: &PersistenceArea{Start: 0, Count: 8}},
	}
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate identity error, got %v", err)
	}
}

func TestPersistencePlanResolvesAreas(t *testing.T) {
	cfg := validPersistenceConfig()
	cfg.Persistence.Ranges = []PersistenceRange{{Port: 502, UnitID: 1,
		Coils:       &PersistenceArea{Start: 0, Count: 8},
		HoldingRegs: &PersistenceArea{Start: 4, Count: 8},
	}}
	plan, err := BuildPersistencePlan(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	areas := plan.Ranges[id]
	if len(areas) != 2 {
		t.Fatalf("expected 2 resolved areas, got %+v", areas)
	}
	if areas[0].Area != memorycore.AreaCoils || areas[0].Start != 0 || areas[0].Count != 8 {
		t.Fatalf("coils resolution wrong: %+v", areas[0])
	}
	if areas[1].Area != memorycore.AreaHoldingRegs || areas[1].Start != 4 || areas[1].Count != 8 {
		t.Fatalf("holding regs resolution wrong: %+v", areas[1])
	}
}

func TestSnapshotFileNamesAreDeterministicAndDistinct(t *testing.T) {
	a := SnapshotFileName(memorycore.MemoryID{Port: 502, UnitID: 1})
	b := SnapshotFileName(memorycore.MemoryID{Port: 502, UnitID: 1})
	c := SnapshotFileName(memorycore.MemoryID{Port: 503, UnitID: 1})
	if a != b {
		t.Fatalf("non-deterministic file name: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("identity collision: %q", a)
	}
	if got := SnapshotPath("/var/lib/mma2", memorycore.MemoryID{Port: 502, UnitID: 1}); got != "/var/lib/mma2/"+a {
		t.Fatalf("unexpected path %q", got)
	}
}
