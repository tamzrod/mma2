package config

import (
	"testing"

	"mma2/internal/memorycore"
)

func boolPtr(v bool) *bool {
	return &v
}

func uint8Ptr(v uint8) *uint8 {
	return &v
}

func stateSealingTestConfig(ss *StateSealingConfig) *Config {
	return &Config{
		Ingress: []IngressGate{
			{
				ID:     "main",
				Listen: ":502",
				Memory: []MemoryDefinition{
					{
						UnitID: 1,
						Coils: Area{
							Start: 0,
							Count: 4,
						},
						HoldingRegs: Area{
							Start: 0,
							Count: 2,
						},
						StateSealing: ss,
					},
				},
			},
		},
	}
}

func TestValidateStateSealingRejectsUnsupportedException(t *testing.T) {
	cfg := stateSealingTestConfig(&StateSealingConfig{
		Area:      "coil",
		Address:   0,
		Exception: uint8Ptr(0x07),
	})

	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for unsupported state sealing exception")
	}
}

func TestValidateStateSealingDisabledSkipsSealingValidation(t *testing.T) {
	cfg := stateSealingTestConfig(&StateSealingConfig{
		Enabled: boolPtr(false),
		Area:    "invalid",
		Address: 99,
	})

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected disabled state sealing to skip validation, got %v", err)
	}
}

func TestBuildMemoryStoreStateSealingDefaultsException(t *testing.T) {
	cfg := stateSealingTestConfig(&StateSealingConfig{
		Area:    "coil",
		Address: 0,
	})

	store, err := BuildMemoryStore(cfg)
	if err != nil {
		t.Fatalf("build memory store: %v", err)
	}

	mem, ok := store.Get(memorycore.MemoryID{Port: 502, UnitID: 1})
	if !ok {
		t.Fatal("expected memory in store")
	}

	seal := mem.StateSealing()
	if seal == nil {
		t.Fatal("expected state sealing to be attached")
	}
	if seal.ExceptionCode != defaultStateSealingExceptionCode {
		t.Fatalf("expected default exception 0x%02X, got 0x%02X", defaultStateSealingExceptionCode, seal.ExceptionCode)
	}
}

func TestBuildMemoryStoreStateSealingConfiguredException(t *testing.T) {
	cfg := stateSealingTestConfig(&StateSealingConfig{
		Area:      "coil",
		Address:   0,
		Exception: uint8Ptr(0x0B),
	})

	store, err := BuildMemoryStore(cfg)
	if err != nil {
		t.Fatalf("build memory store: %v", err)
	}

	mem, ok := store.Get(memorycore.MemoryID{Port: 502, UnitID: 1})
	if !ok {
		t.Fatal("expected memory in store")
	}

	seal := mem.StateSealing()
	if seal == nil {
		t.Fatal("expected state sealing to be attached")
	}
	if seal.ExceptionCode != 0x0B {
		t.Fatalf("expected configured exception 0x0B, got 0x%02X", seal.ExceptionCode)
	}
}

func TestBuildMemoryStoreStateSealingDisabled(t *testing.T) {
	cfg := stateSealingTestConfig(&StateSealingConfig{
		Enabled: boolPtr(false),
		Area:    "coil",
		Address: 0,
	})

	store, err := BuildMemoryStore(cfg)
	if err != nil {
		t.Fatalf("build memory store: %v", err)
	}

	mem, ok := store.Get(memorycore.MemoryID{Port: 502, UnitID: 1})
	if !ok {
		t.Fatal("expected memory in store")
	}
	if mem.StateSealing() != nil {
		t.Fatal("expected disabled state sealing to be omitted from memory")
	}
}
