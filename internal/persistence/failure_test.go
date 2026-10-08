package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// TestFlushFailureIsObservableAndRetainsDirty makes the primary snapshot
// unwritable and verifies the manager enters FAILED, records the error, keeps
// the unflushed dirty ranges, and never modifies the known-good backup.
func TestFlushFailureIsObservableAndRetainsDirty(t *testing.T) {
	dir := t.TempDir()
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem := newMemory(t)
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)

	primary := config.SnapshotPath(dir, id)
	backup := BackupPath(primary)
	backupBefore, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}

	// Make the primary unwritable by replacing it with a directory: opening it
	// O_RDWR fails with EISDIR regardless of process privileges.
	if err := os.Remove(primary); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(primary, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewScheduler(m)
	s.delay = 5 * time.Millisecond
	s.checkpt = time.Hour
	s.Start(context.Background())

	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0xAA, 0xBB}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	if !m.Failed() {
		t.Fatalf("state = %v, want FAILED after failed flush", m.State())
	}
	if m.LastError() == nil {
		t.Fatal("failure error not recorded")
	}
	if m.Diagnostics().LastError == "" {
		t.Fatal("diagnostics missing last error")
	}
	// Unflushed data must remain visible.
	if ranges := m.DirtyRanges(id); len(ranges) == 0 {
		t.Fatal("dirty ranges lost after failed flush")
	}
	// The known-good backup must be untouched.
	backupAfter, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupBefore) != string(backupAfter) {
		t.Fatal("backup modified during a failing flush")
	}
}

// TestFlushRetriesStopOnceFailed verifies the scheduler does not keep rewriting
// after FAILED.
func TestFlushRetriesStopOnceFailed(t *testing.T) {
	dir := t.TempDir()
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem := newMemory(t)
	if err := m.RestoreMemory(id, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(id, mem)

	primary := config.SnapshotPath(dir, id)
	if err := os.Remove(primary); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(primary, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewScheduler(m)
	s.delay = 5 * time.Millisecond
	s.checkpt = time.Hour

	_ = mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0x01, 0x02})
	s.flushAll()
	if !m.Failed() {
		t.Fatal("expected FAILED")
	}
	// After FAILED, flushAll is a no-op and must not panic or clear dirty data.
	before := m.DirtyRanges(id)
	s.flushAll()
	after := m.DirtyRanges(id)
	if len(before) != len(after) {
		t.Fatalf("dirty ranges changed after FAILED: %+v -> %+v", before, after)
	}
}

// TestCheckpointSkipsInvalidPrimary verifies a corrupt primary never
// overwrites the good backup.
func TestCheckpointSkipsInvalidPrimary(t *testing.T) {
	dir := t.TempDir()
	m, err := New(restorePlan(dir), allocations())
	if err != nil {
		t.Fatal(err)
	}
	id := memorycore.MemoryID{Port: 502, UnitID: 1}
	if err := m.RestoreMemory(id, newMemory(t)); err != nil {
		t.Fatal(err)
	}

	primary := config.SnapshotPath(dir, id)
	backup := BackupPath(primary)
	backupBefore, _ := os.ReadFile(backup)

	// Corrupt the primary, then force a checkpoint.
	data, _ := os.ReadFile(primary)
	data[0] ^= 0xFF
	_ = os.WriteFile(primary, data, 0o600)

	s := NewScheduler(m)
	s.checkpt = 0
	s.maybeCheckpoint(id)

	backupAfter, _ := os.ReadFile(backup)
	if string(backupBefore) != string(backupAfter) {
		t.Fatal("invalid primary overwrote the good backup")
	}
}

func TestFailedStateSticky(t *testing.T) {
	m, err := New(restorePlan(t.TempDir()), allocations())
	if err != nil {
		t.Fatal(err)
	}
	m.markFailed(errBoom, time.Now())
	if !m.Failed() {
		t.Fatal("not FAILED")
	}
	m.markSaved(time.Now())
	if !m.Failed() {
		t.Fatal("markSaved cleared FAILED state")
	}
}

var errBoom = os.ErrClosed
