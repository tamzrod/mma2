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

func TestReplaceAtomicCreatesValidSnapshot(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 502, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	img := buildImage(t, l)

	if err := store.ReplaceAtomic(img); err != nil {
		t.Fatalf("ReplaceAtomic: %v", err)
	}
	got, err := store.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLayout(got); err != nil {
		t.Fatalf("stored snapshot not parseable: %v", err)
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

func TestWriteWordTargetedUpdate(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}

	off, err := l.RegisterOffset(memorycore.AreaHoldingRegs, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteWord(off, 0xBEEF); err != nil {
		t.Fatalf("WriteWord: %v", err)
	}

	data, err := store.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// The targeted register changed; every other payload byte is unchanged.
	if data[off] != 0xBE || data[off+1] != 0xEF {
		t.Fatalf("targeted register not written: %x", data[off:off+2])
	}
	for i := l.PayloadStart; i < l.TotalSize; i++ {
		if uint32(i) == off || uint32(i) == off+1 {
			continue
		}
		if data[i] != 0 {
			t.Fatalf("unexpected payload byte change at %d: %x", i, data[i])
		}
	}
	// Header must remain valid after a targeted write.
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("header invalid after targeted write: %v", err)
	}
}

func TestWriteBitBytePreservesAdjacentBits(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}

	// Set bit 2, then set bit 5 in the same containing byte. Bit 2 must survive.
	byteOff, bit2, err := l.BitByteOffset(memorycore.AreaCoils, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, bit5, err := l.BitByteOffset(memorycore.AreaCoils, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteBitByte(byteOff, bit2, true); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteBitByte(byteOff, bit5, true); err != nil {
		t.Fatal(err)
	}
	data, _ := store.ReadAll()
	want := byte(1<<2 | 1<<5)
	if data[byteOff] != want {
		t.Fatalf("containing byte = %08b, want %08b", data[byteOff], want)
	}

	// Clearing bit 2 leaves bit 5.
	if err := store.WriteBitByte(byteOff, bit2, false); err != nil {
		t.Fatal(err)
	}
	data, _ = store.ReadAll()
	if data[byteOff] != byte(1<<5) {
		t.Fatalf("after clear, byte = %08b, want %08b", data[byteOff], 1<<5)
	}
}

func TestExists(t *testing.T) {
	store := tempStore(t)
	ok, err := store.Exists()
	if err != nil || ok {
		t.Fatalf("Exists before write = %v,%v", ok, err)
	}
	l, _ := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}
	ok, err = store.Exists()
	if err != nil || !ok {
		t.Fatalf("Exists after write = %v,%v", ok, err)
	}
}

func TestConcurrentTargetedWritesSerialized(t *testing.T) {
	store := tempStore(t)
	l, err := NewLayout(memorycore.MemoryID{Port: 1, UnitID: 1}, sampleSegments())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceAtomic(buildImage(t, l)); err != nil {
		t.Fatal(err)
	}

	byteOff, _, err := l.BitByteOffset(memorycore.AreaCoils, 3)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(set bool) {
			defer wg.Done()
			_ = store.WriteBitByte(byteOff, 3, set)
		}(i%2 == 0)
	}
	wg.Wait()

	data, err := store.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLayout(data); err != nil {
		t.Fatalf("snapshot corrupted by concurrent writes: %v", err)
	}
}
