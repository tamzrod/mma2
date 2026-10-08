package persistence

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore performs serialized, positional disk access for one snapshot
// identity: a primary file and a separately named backup file.
//
// Write modes:
//
//   - ReplaceAtomic(image): writes a whole snapshot image to a temporary file,
//     fsyncs, then renames it over the primary. Used for initial creation and
//     rebuilds; atomic at the file level.
//   - InstallBackup(image): the same atomic replacement for the backup file.
//     Callers must only pass an image that has passed ParseLayout validation.
//   - ApplyWord / ApplyBitByte: positional data write plus the recomputed CRC of
//     the touched fixed-size block. Unaffected payload and CRCs are preserved,
//     so one register change never rewrites or rehashes the whole file.
//
// Durability and crash consistency (explicit contract):
//   - Targeted writes are visible to subsequent reads immediately (page cache)
//     but durable across power loss only after an explicit Sync. There is no
//     implied per-write durability.
//   - An in-place data+CRC write is identified by CRC32 but is NOT itself
//     atomic. The block CRC catches a torn write at startup; recovery uses the
//     backup rather than exposing possibly-corrupt primary data.
//   - The backup is a known-good image. It is only (re)created from a primary
//     that has already passed integrity validation, so the only good copy is
//     never overwritten with unverified data.
//
// No lock file is taken; two processes sharing one path will corrupt each other
// and must be given distinct directories.
type FileStore struct {
	mu         sync.Mutex
	path       string
	backupPath string
	layout     *Layout
}

// NewFileStore returns a store for a snapshot path. The backup path is derived
// from the primary path by replacing the extension with ".bak".
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path, backupPath: BackupPath(path)}
}

// BackupPath returns the backup path for a snapshot path.
func BackupPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".bak"
	}
	return strings.TrimSuffix(path, ext) + ".bak"
}

// Path returns the primary snapshot file path.
func (s *FileStore) Path() string { return s.path }

// BackupFilePath returns the backup snapshot file path.
func (s *FileStore) BackupFilePath() string { return s.backupPath }

// SetLayout records the layout used for targeted CRC updates.
func (s *FileStore) SetLayout(l *Layout) { s.layout = l }

// ReplaceAtomic writes image to a temporary file, fsyncs it, then renames it
// over the primary path.
func (s *FileStore) ReplaceAtomic(image []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceAtomicLocked(s.path, image)
}

// InstallBackup atomically installs a validated image as the backup.
func (s *FileStore) InstallBackup(image []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceAtomicLocked(s.backupPath, image)
}

// ReplaceBoth atomically installs img as both the primary and the backup.
// Used to establish a known-good baseline at initial creation/rebuild.
func (s *FileStore) ReplaceBoth(img []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.replaceAtomicLocked(s.path, img); err != nil {
		return err
	}
	return s.replaceAtomicLocked(s.backupPath, img)
}

func (s *FileStore) replaceAtomicLocked(path string, image []byte) error {
	dir := filepath.Dir(path)
	if err := cleanStaleTemps(dir, filepath.Base(path)); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("persistence: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(image); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: rename: %w", err)
	}
	return syncDir(dir)
}

// ApplyWord writes a 2-byte big-endian register value at byte offset and
// refreshes the CRC of the affected block(s), preserving all other bytes.
func (s *FileStore) ApplyWord(offset uint32, value uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyLocked(offset, []byte{byte(value >> 8), byte(value)})
}

// ApplyBitByte applies set/clear of one bit within the byte at byteOffset and
// refreshes the CRC of the affected block. Adjacent bits are preserved.
func (s *FileStore) ApplyBitByte(byteOffset uint32, bit uint8, set bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("persistence: open snapshot: %w", err)
	}
	current := make([]byte, 1)
	n, err := f.ReadAt(current, int64(byteOffset))
	if err != nil && n == 0 {
		current[0] = 0
	} else if n != 1 {
		_ = f.Close()
		return fmt.Errorf("persistence: short read at offset %d", byteOffset)
	}
	_ = f.Close()

	mask := byte(1 << bit)
	if set {
		current[0] |= mask
	} else {
		current[0] &^= mask
	}
	return s.applyLocked(byteOffset, current)
}

// ApplyRange writes an arbitrary byte span at offset and refreshes the CRC of
// every block it touches, preserving all other bytes.
func (s *FileStore) ApplyRange(offset uint32, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyLocked(offset, data)
}

// applyLocked writes data at offset and refreshes the CRC of every touched
// block.
func (s *FileStore) applyLocked(offset uint32, data []byte) error {
	if s.layout == nil {
		return fmt.Errorf("persistence: layout not set on store")
	}

	f, err := os.OpenFile(s.path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("persistence: open snapshot: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteAt(data, int64(offset)); err != nil {
		return fmt.Errorf("persistence: write at %d: %w", offset, err)
	}

	// Recompute and rewrite the CRC of every block overlapping [offset, end).
	payloadLen := s.layout.payloadSize()
	payloadOffset := offset - s.layout.PayloadStart
	first, last := blockRange(s.layout, payloadOffset, uint32(len(data)))
	block := make([]byte, s.layout.BlockSize)
	for i := first; i <= last; i++ {
		blockStart := uint64(i) * uint64(s.layout.BlockSize)
		blockEnd := blockStart + uint64(s.layout.BlockSize)
		if blockEnd > uint64(payloadLen) {
			blockEnd = uint64(payloadLen)
		}
		n := int(blockEnd - blockStart)
		if _, err := f.ReadAt(block[:n], int64(s.layout.PayloadStart)+int64(blockStart)); err != nil {
			return fmt.Errorf("persistence: read block %d: %w", i, err)
		}
		crc := make([]byte, blockCRCEntrySize)
		binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(block[:n]))
		if _, err := f.WriteAt(crc, int64(s.layout.blockCRCPos(i))); err != nil {
			return fmt.Errorf("persistence: write block %d CRC: %w", i, err)
		}
	}

	return f.Sync()
}

// payloadSize returns the total payload byte length for the layout.
func (l *Layout) payloadSize() uint32 { return l.TotalSize - l.PayloadStart }

// blockRange returns the inclusive block index range covering a payload span.
func blockRange(l *Layout, payloadOffset, length uint32) (uint32, uint32) {
	if length == 0 {
		return 0, 0
	}
	bs := uint32(l.BlockSize)
	return payloadOffset / bs, (payloadOffset + length - 1) / bs
}

// Sync flushes the primary snapshot file to stable storage.
func (s *FileStore) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("persistence: open snapshot: %w", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("persistence: sync snapshot: %w", err)
	}
	return nil
}

// ReadPrimary returns the whole primary snapshot image.
func (s *FileStore) ReadPrimary() ([]byte, error) { return s.readFile(s.path) }

// ReadBackup returns the whole backup snapshot image.
func (s *FileStore) ReadBackup() ([]byte, error) { return s.readFile(s.backupPath) }

func (s *FileStore) readFile(path string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// PrimaryExists reports whether the primary snapshot file exists.
func (s *FileStore) PrimaryExists() (bool, error) { return s.fileExists(s.path) }

// BackupExists reports whether the backup snapshot file exists.
func (s *FileStore) BackupExists() (bool, error) { return s.fileExists(s.backupPath) }

func (s *FileStore) fileExists(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// cleanStaleTemps removes leftover temp files for this target from dir.
func cleanStaleTemps(dir, base string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("persistence: read dir: %w", err)
	}
	prefix := base + ".tmp-"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), prefix) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return fmt.Errorf("persistence: remove stale temp %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

// syncDir fsyncs a directory so a rename is durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("persistence: open dir: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("persistence: sync dir: %w", err)
	}
	return nil
}
