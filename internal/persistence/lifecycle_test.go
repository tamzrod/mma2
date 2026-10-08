package persistence

import (
	"errors"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

func testTime() time.Time { return time.Unix(1700000000, 0).UTC() }

func planFor() *config.ResolvedPersistence {
	return &config.ResolvedPersistence{
		Directory: "/var/lib/mma2",
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {
				{Area: memorycore.AreaCoils, Start: 0, Count: 8},
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 16},
			},
		},
	}
}

func TestNewDisabled(t *testing.T) {
	m, err := New(nil, allocations())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != StateDisabled {
		t.Fatalf("state = %v, want DISABLED", m.State())
	}
	if m.Enabled() {
		t.Fatal("disabled manager reports enabled")
	}
	d := m.Diagnostics()
	if d.State != StateDisabled || d.Identities != 0 || d.Segments != 0 {
		t.Fatalf("unexpected diagnostics: %+v", d)
	}
}

func TestNewEnabledStartsRestoring(t *testing.T) {
	m, err := New(planFor(), allocations())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.State() != StateRestoring {
		t.Fatalf("state = %v, want RESTORING", m.State())
	}
	if !m.Enabled() {
		t.Fatal("enabled manager reports disabled")
	}
	d := m.Diagnostics()
	if d.Identities != 1 || d.Segments != 2 || d.Directory != "/var/lib/mma2" {
		t.Fatalf("unexpected diagnostics: %+v", d)
	}
	segs, ok := m.Segments(memorycore.MemoryID{Port: 502, UnitID: 1})
	if !ok || len(segs) != 2 {
		t.Fatalf("segments not retained: %+v ok=%v", segs, ok)
	}
	if _, ok := m.Segments(memorycore.MemoryID{Port: 999, UnitID: 1}); ok {
		t.Fatal("unknown identity returned segments")
	}
}

func TestLifecycleTransitions(t *testing.T) {
	m, err := New(planFor(), allocations())
	if err != nil {
		t.Fatal(err)
	}

	m.markRestored(testTime())
	if m.State() != StateReady {
		t.Fatalf("state = %v, want READY", m.State())
	}
	m.markSaved(testTime())
	if d := m.Diagnostics(); d.LastRestoreAt.IsZero() || d.LastSaveAt.IsZero() {
		t.Fatalf("timestamps not recorded: %+v", d)
	}

	boom := errors.New("disk full")
	m.markFailed(boom, testTime())
	if m.State() != StateFailed {
		t.Fatalf("state = %v, want FAILED", m.State())
	}
	d := m.Diagnostics()
	if d.LastError != "disk full" || d.LastErrorAt.IsZero() {
		t.Fatalf("failure not recorded: %+v", d)
	}
}

func TestStateStrings(t *testing.T) {
	cases := map[State]string{
		StateDisabled:  "DISABLED",
		StateRestoring: "RESTORING",
		StateReady:     "READY",
		StateFailed:    "FAILED",
		State(99):      "UNKNOWN",
	}
	for s, want := range cases {
		if s.String() != want {
			t.Fatalf("State(%d).String() = %q, want %q", s, s.String(), want)
		}
	}
}

func TestNilManagerSafe(t *testing.T) {
	var m *Manager
	if m.Enabled() {
		t.Fatal("nil manager enabled")
	}
	if m.State() != StateDisabled {
		t.Fatal("nil manager not DISABLED")
	}
	if _, ok := m.Segments(memorycore.MemoryID{}); ok {
		t.Fatal("nil manager returned segments")
	}
	if m.Diagnostics().State != StateDisabled {
		t.Fatal("nil manager diagnostics not DISABLED")
	}
}
