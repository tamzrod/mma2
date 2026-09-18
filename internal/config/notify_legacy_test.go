package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"mma2/internal/notify"
)

func notifyFixturePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "test", "test_notify.yaml")
}

func TestLegacyNotifyFixtureLoadsWithoutRBE(t *testing.T) {
	cfg, err := Load(notifyFixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.RBE != nil {
		t.Fatal("fixture must not declare rbe")
	}
	if cfg.Notify != nil && cfg.Notify.Influx != nil {
		t.Fatal("fixture must not declare notify.influx")
	}
	rules, err := BuildRBERules(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("unexpected rbe rules: %+v", rules)
	}
	reg, err := BuildNotifyRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Rules()
	if len(got) != 3 {
		t.Fatalf("want 3 notify rules, got %d: %+v", len(got), got)
	}
	var coils, hr int
	for _, r := range got {
		if r.Port != 1502 || r.UnitID != 1 {
			t.Fatalf("identity: %+v", r)
		}
		switch r.Area {
		case notify.AreaCoils:
			coils++
		case notify.AreaHoldingRegisters:
			hr++
		}
	}
	if coils != 1 || hr != 2 {
		t.Fatalf("areas coils=%d hr=%d", coils, hr)
	}
}
