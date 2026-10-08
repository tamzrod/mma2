package persistence

import (
	"bytes"
	"os"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// Regression: the first dirty flush must not refresh the verified backup.
// The first refresh is allowed only after the checkpoint interval elapses.
func TestSchedulerFirstBackupWaitsForInterval(t *testing.T) {
	_, s, dir, id, mem := scheduleManager(t)
	primary := config.SnapshotPath(dir, id)
	backup := BackupPath(primary)

	before, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}

	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 5, 1, []byte{0xBE, 0xEF}); err != nil {
		t.Fatal(err)
	}
	s.flushIdentity(id)

	latest, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(latest, before) {
		t.Fatal("primary did not receive targeted update")
	}
	actualBackup, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actualBackup, before) {
		t.Fatal("first dirty flush overwrote known-good backup before interval")
	}

	// Simulate the elapsed interval without sleeping.
	s.lastCkpt[id] = time.Now().Add(-s.checkpt - time.Second)
	s.maybeCheckpoint(id)
	refreshed, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(refreshed, latest) {
		t.Fatal("due backup checkpoint did not capture valid primary")
	}
}
