package persistence

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"mma2/internal/memorycore"
)

func tempStore(t *testing.T) *FileStore {
	t.Helper()
	return NewFileStore(filepath.Join(t.TempDir(), "snap.bin"))
}

func buildImage(t *testing.T, l *Layout) []byte {
	t.Helper()
	img, err := l.BuildSnapshot(func(seg SegmentLayout) ([]byte, error) {
		return make([]byte, seg.Length), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestBackupPath(t *testing.T) {
	if got := BackupPath("/x/mma2-502-1.bin"); got != "/x/mma2-502-1.bak" {
		t.Fatalf("BackupPath = %q", got)
	}
	if got := BackupPath("/x/noext"); got != "/x/noext.bak" {
		t.Fatalf("BackupPath = %q", got)
	}
}

func TestReplaceAtomicCreatesValidSnapshot(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 502, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatalf("ReplaceAtomic: %v", err)
	}
	got, err := store.ReadPrimary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLayout(got); err != nil {
		t.Fatalf("stored snapshot not parseable: %v", err)
	}
}

func TestReplaceBothAndInstallBackup(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	img := buildImage(t, l)
	if err := store.ReplaceBoth(img); err != nil {
		t.Fatalf("ReplaceBoth: %v", err)
	}
	if ok, _ := store.PrimaryExists(); !ok {
		t.Fatal("primary missing after ReplaceBoth")
	}
	if ok, _ := store.BackupExists(); !ok {
		t.Fatal("backup missing after ReplaceBoth")
	}

	if err := store.InstallBackup(img); err != nil {
		t.Fatalf("InstallBackup: %v", err)
	}
}

func TestReplaceAtomicCleansStaleTemps(t *testing.T) {
	store := tempStore(t)
	dir := filepath.Dir(store.Path())
	stale := filepath.Join(dir, filepath.Base(store.Path())+".tmp-123")
	if err := os.WriteFile(stale, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, _ := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp not cleaned: %v", err)
	}
}

func TestApplyWordTargetedUpdate(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	store.SetLayout(l)
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}

	off, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyWord(off, 0xBEEF); err != nil {
		t.Fatalf("ApplyWord: %v", err)
	}

	data, err := store.ReadPrimary()
	if err != nil {
		t.Fatal(err)
	}
	if data[off] != 0xBE || data[off+1] != 0xEF {
		t.Fatalf("targeted register not written: %x", data[off:off+2])
	}
	// Unaffected payload bytes are untouched.
	for i := l.PayloadStart; i < l.TotalSize; i++ {
		if uint32(i) == off || uint32(i) == off+1 {
			continue
		}
		if data[i] != 0 {
			t.Fatalf("unexpected payload byte change at %d: %x", i, data[i])
		}
	}
	// Targeted update must leave the file fully valid (data + block CRC).
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("snapshot invalid after targeted write: %v", err)
	}
}

func TestApplyWordPreservesOtherBlocks(t *testing.T) {
	store := tempStore(t)
	// Build a layout with a payload larger than one block so multiple blocks exist.
	segs := []Segment{{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 200}}
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, segs)
	if err != nil {
		t.Fatal(err)
	}
	if l.BlockCount < 2 {
		t.Fatalf("expected multiple blocks, got %d", l.BlockCount)
	}
	store.SetLayout(l)
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}
	before, _ := store.ReadPrimary()
	crcBefore := append([]byte(nil), before[l.BlockCRCOffset:l.PayloadStart]...)

	off, _ := l.RegisterOffset(memorycore.AreaHoldingRegs, 1) // within first block
	if err := store.ApplyWord(off, 0x1234); err != nil {
		t.Fatal(err)
	}
	after, _ := store.ReadPrimary()
	crcAfter := after[l.BlockCRCOffset:l.PayloadStart]

	// Only the first block's CRC may change.
	for i := 0; i < len(crcBefore); i += blockCRCEntrySize {
		block := i / blockCRCEntrySize
		same := string(crcBefore[i:i+blockCRCEntrySize]) == string(crcAfter[i:i+blockCRCEntrySize])
		if block == 0 && same {
			t.Fatalf("block 0 CRC did not change")
		}
		if block != 0 && !same {
			t.Fatalf("block %d CRC changed unexpectedly", block)
		}
	}
}

func TestApplyBitBytePreservesAdjacentBits(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	store.SetLayout(l)
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}

	byteOff, bit2, err := l.BitByteOffset(memorycore.AreaCoils, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, bit5, err := l.BitByteOffset(memorycore.AreaCoils, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyBitByte(byteOff, bit2, true); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyBitByte(byteOff, bit5, true); err != nil {
		t.Fatal(err)
	}
	data, _ := store.ReadPrimary()
	if data[byteOff] != byte(1<<2|1<<5) {
		t.Fatalf("containing byte = %08b", data[byteOff])
	}
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("snapshot invalid after bit update: %v", err)
	}

	if err := store.ApplyBitByte(byteOff, bit2, false); err != nil {
		t.Fatal(err)
	}
	data, _ = store.ReadPrimary()
	if data[byteOff] != byte(1<<5) {
		t.Fatalf("after clear, byte = %08b", data[byteOff])
	}
}

func TestConcurrentTargetedWritesSerialized(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	store.SetLayout(l)
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}
	byteOff, _, _ := l.BitByteOffset(memorycore.AreaCoils, 3)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(set bool) {
			defer wg.Done()
			_ = store.ApplyBitByte(byteOff, 3, set)
		}(i%2 == 0)
	}
	wg.Wait()

	data, err := store.ReadPrimary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("snapshot corrupted by concurrent writes: %v", err)
	}
}
