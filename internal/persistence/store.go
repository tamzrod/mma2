package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore performs serialized, positional disk access for one snapshot file.
//
// Two write modes exist:
//
//   - Replace (ReplaceAtomic): writes a whole snapshot image to a temporary file,
//     fsyncs, then renames it over the target. This is used for initial creation
//     and layout rebuilds and is atomic at the file level.
//   - Targeted (WriteWord / WriteBitByte): positional writes to a single 2-byte
//     register or one containing bit byte. These preserve all other bytes but are
//     NOT power-loss atomic on their own; the crash-consistency contract is
//     documented, not assumed.
//
// Targeted writes are visible to subsequent reads immediately (page cache) but
// are only guaranteed durable across power loss after an explicit Sync. There is
// no implied per-write durability; the flush/durability contract is defined by
// the caller's Sync schedule (initial create/rebuild is durable via
// ReplaceAtomic).
//
// All operations are serialized by a mutex so concurrent writers cannot
// interleave their file mutations. No lock file is taken; two processes sharing
// one path will corrupt each other and must be given distinct directories.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore returns a store for the given snapshot path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Path returns the snapshot file path.
func (s *FileStore) Path() string { return s.path }

// ReplaceAtomic writes image to a temporary file, fsyncs it, then renames it
// over the target path. A crash before rename leaves the target untouched; a
// crash after rename leaves the fully written image. Stale temporary files from
// earlier interrupted replacements in this directory are cleaned up first.
func (s *FileStore) ReplaceAtomic(image []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := cleanStaleTemps(dir, filepath.Base(s.path)); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
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
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: rename: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return nil
}

// WriteWord writes a 2-byte big-endian register value at byte offset.
func (s *FileStore) WriteWord(offset uint32, value uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := []byte{byte(value >> 8), byte(value)}
	return s.writeAt(offset, buf)
}

// WriteBitByte applies set/clear of one bit within the byte at byteOffset.
// It reads the current byte (defaulting to 0 if unwritten), updates only bit,
// and writes the byte back. Adjacent bits are preserved.
func (s *FileStore) WriteBitByte(byteOffset uint32, bit uint8, set bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("persistence: open snapshot: %w", err)
	}
	defer f.Close()

	current := make([]byte, 1)
	n, err := f.ReadAt(current, int64(byteOffset))
	if err != nil && n == 0 {
		// A missing byte (sparse or short file) is treated as zero.
		current[0] = 0
	} else if n != 1 {
		return fmt.Errorf("persistence: short read at offset %d", byteOffset)
	}

	mask := byte(1 << bit)
	if set {
		current[0] |= mask
	} else {
		current[0] &^= mask
	}

	if _, err := f.WriteAt(current, int64(byteOffset)); err != nil {
		return fmt.Errorf("persistence: write bit byte: %w", err)
	}
	return f.Sync()
}

// Sync flushes the snapshot file to stable storage.
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

// ReadAll returns the whole snapshot image.
func (s *FileStore) ReadAll() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Exists reports whether the snapshot file exists.
func (s *FileStore) Exists() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := os.Stat(s.path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *FileStore) writeAt(offset uint32, buf []byte) error {
	f, err := os.OpenFile(s.path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("persistence: open snapshot: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteAt(buf, int64(offset)); err != nil {
		return fmt.Errorf("persistence: write at %d: %w", offset, err)
	}
	return f.Sync()
}

// cleanStaleTemps removes leftover temp files for this snapshot from dir.
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
