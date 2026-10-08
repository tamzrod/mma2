package persistence

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// Scheduler owns the background flush loop for all persisted identities.
//
// Contract:
//   - Dirty ranges are coalesced by the observer (P07); the scheduler drains
//     them and applies targeted positional writes plus block CRC(s).
//   - A single goroutine serializes all disk writes; store methods are also
//     internally serialized, so there is exactly one writer at a time.
//   - A bounded coalescing delay batches bursts; the final flush on orderly
//     shutdown drains everything.
//   - Abrupt crash: writes since the last completed flush may be lost. Index
//     changes and backup refresh use atomic whole-file replacement. In-place
//     targeted writes are identified by block CRC and recovered from the backup
//     at startup; no atomicity beyond that is claimed.
type Scheduler struct {
	mgr     *Manager
	delay   time.Duration
	checkpt time.Duration

	mu       sync.Mutex
	wake     chan struct{}
	stop     chan struct{}
	stopped  chan struct{}
	lastCkpt map[memorycore.MemoryID]time.Time
}

// DefaultFlushDelay is the bounded coalescing delay before a dirty identity is
// flushed.
const DefaultFlushDelay = 50 * time.Millisecond

// BackupInterval is the fixed cadence at which the known-good backup is
// refreshed from a validated primary image. Locked to 60 seconds by contract:
// a simple fixed interval, not a per-mutation or multi-generation scheme.
const BackupInterval = 60 * time.Second

// NewScheduler creates a scheduler for the manager. It is a no-op when
// persistence is disabled.
func NewScheduler(mgr *Manager) *Scheduler {
	if mgr == nil || !mgr.Enabled() {
		return &Scheduler{stopped: closedChan()}
	}
	// An initial verified backup already exists after RestoreMemory.
	// Start the 60-second clock now, not on the first dirty flush.
	lastCkpt := make(map[memorycore.MemoryID]time.Time)
	started := time.Now()
	for _, id := range mgr.PersistedIdentities() {
		lastCkpt[id] = started
	}
	return &Scheduler{
		mgr:      mgr,
		delay:    DefaultFlushDelay,
		checkpt:  BackupInterval,
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
		lastCkpt: lastCkpt,
	}
}

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// Start launches the background flush loop.
func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.wake == nil {
		return
	}
	go s.run(ctx)
}

// Notify signals that dirty data may be available. It never blocks.
func (s *Scheduler) Notify() {
	if s == nil || s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.stopped)

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	armed := false

	// Periodic checkpoint even when idle, so the backup cadence holds without
	// new writes.
	ticker := time.NewTicker(BackupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.flushAll()
			return
		case <-s.stop:
			s.flushAll()
			return
		case <-ticker.C:
			s.flushAll()
		case <-s.wake:
			// Arm once on the first write in a burst. Repeated writes must
			// never postpone the maximum dirty-to-flush delay.
			if !armed {
				timer.Reset(s.delay)
				armed = true
			}
		case <-timer.C:
			armed = false
			s.flushAll()
		}
	}
}

// Close stops the loop and performs a final flush. Idempotent. A failed final
// flush leaves the manager FAILED (recorded by flushIdentity); the backup is
// never touched by a failing flush.
func (s *Scheduler) Close() {
	if s == nil || s.stop == nil {
		return
	}
	s.mu.Lock()
	select {
	case <-s.stop:
		s.mu.Unlock()
		return
	default:
		close(s.stop)
	}
	s.mu.Unlock()
	<-s.stopped
}

// flushAll drains and flushes every identity with pending dirty ranges. Once
// the manager is FAILED it stops attempting flushes until an explicit recovery
// path resets state; this avoids repeated failed writes and false success.
func (s *Scheduler) flushAll() {
	if s.mgr.Failed() {
		return
	}
	for _, id := range s.mgr.PersistedIdentities() {
		s.flushIdentity(id)
	}
}

// flushIdentity drains and applies the dirty ranges for one identity. On a
// failure the manager becomes FAILED and the drained ranges are re-marked so
// the lost data remains visible and is not silently forgotten. The known-good
// backup is never modified here.
func (s *Scheduler) flushIdentity(id memorycore.MemoryID) {
	ranges, _ := s.mgr.DirtySnapshot(id)
	if len(ranges) > 0 {
		if err := s.applyRanges(id, ranges); err != nil {
			s.mgr.remarkDirty(id, ranges)
			wrapped := fmt.Errorf("persistence: identity (port=%d unit=%d): flush failed: %w", id.Port, id.UnitID, err)
			log.Printf("%v (persistence entering FAILED; unflushed data retained)", wrapped)
			s.mgr.markFailed(wrapped, time.Now())
			return
		}
		s.mgr.markSaved(time.Now())
	}
	s.maybeCheckpoint(id)
}

// applyRanges reads the current bytes for each dirty range from memory and
// writes them to the primary snapshot with refreshed block CRCs.
func (s *Scheduler) applyRanges(id memorycore.MemoryID, ranges []DirtyRange) error {
	layout, ok := s.mgr.LayoutFor(id)
	if !ok {
		return nil
	}
	mem, ok := s.mgr.memory(id)
	if !ok {
		return nil
	}
	store := NewFileStore(config.SnapshotPath(s.mgr.Directory(), id))
	store.SetLayout(layout)

	for _, r := range ranges {
		data, err := readFileSpan(mem, layout, r)
		if err != nil {
			return err
		}
		if err := store.ApplyRange(r.Start, data); err != nil {
			return err
		}
	}
	return nil
}

// readFileSpan reads the current memory values corresponding to a file byte
// range, using the layout to translate file offsets back to memory addresses.
func readFileSpan(mem *memorycore.Memory, layout *Layout, r DirtyRange) ([]byte, error) {
	seg, local, ok := layout.segmentContaining(r.Start)
	if !ok {
		return nil, errNoSegment
	}
	data := make([]byte, r.Length)
	if seg.Area.IsRegArea() {
		regIndex := local / 2
		regCount := r.Length / 2
		return readRegSpanInto(mem, seg, regIndex, regCount, data)
	}
	return readBitSpanInto(mem, seg, local, r.Length, data)
}

// segmentContaining returns the segment containing a file byte offset and the
// byte offset within that segment.
func (l *Layout) segmentContaining(offset uint32) (SegmentLayout, uint32, bool) {
	for _, seg := range l.Segments {
		if offset >= seg.Offset && offset < seg.Offset+seg.Length {
			return seg, offset - seg.Offset, true
		}
	}
	return SegmentLayout{}, 0, false
}

// readRegSpanInto reads whole registers covering [local, local+len) into dst.
func readRegSpanInto(mem *memorycore.Memory, seg SegmentLayout, regIndex, regCount uint32, dst []byte) ([]byte, error) {
	addr := uint16(uint32(seg.Start) + regIndex)
	buf := make([]byte, regCount*2)
	if err := mem.ReadRegs(seg.Area, addr, uint16(regCount), buf); err != nil {
		return nil, err
	}
	copy(dst, buf)
	return dst, nil
}

// readBitSpanInto reads the byte-aligned bit span covering the range into dst.
// When the range's final byte extends past the segment's bit count (a
// non-byte-aligned bit segment), only the in-range bits are read and the unused
// high bits of the destination byte are left zero, matching the always-zero
// padding bits stored on disk.
func readBitSpanInto(mem *memorycore.Memory, seg SegmentLayout, local, length uint32, dst []byte) ([]byte, error) {
	firstBit := uint32(seg.Start) + local*8
	count := length * 8
	segEnd := uint32(seg.Start) + uint32(seg.Count)
	if firstBit >= segEnd {
		return dst, nil
	}
	if firstBit+count > segEnd {
		count = segEnd - firstBit
	}
	buf := make([]byte, (count+7)/8)
	if err := mem.ReadBits(seg.Area, uint16(firstBit), uint16(count), buf); err != nil {
		return nil, err
	}
	copy(dst, buf)
	return dst, nil
}

// maybeCheckpoint refreshes the known-good backup from a validated primary when
// the checkpoint interval has elapsed.
func (s *Scheduler) maybeCheckpoint(id memorycore.MemoryID) {
	last, ok := s.lastCkpt[id]
	now := time.Now()
	if ok && now.Sub(last) < s.checkpt {
		return
	}
	layout, ok := s.mgr.LayoutFor(id)
	if !ok {
		return
	}
	store := NewFileStore(config.SnapshotPath(s.mgr.Directory(), id))
	store.SetLayout(layout)

	data, err := store.ReadPrimary()
	if err != nil {
		return
	}
	if _, err := ParseLayout(data); err != nil {
		// Primary invalid: do not overwrite the good backup with bad data.
		return
	}
	if err := store.InstallBackup(data); err != nil {
		return
	}
	s.mu.Lock()
	s.lastCkpt[id] = now
	s.mu.Unlock()
}

var errNoSegment = errNoSegmentError{}

type errNoSegmentError struct{}

func (errNoSegmentError) Error() string { return "persistence: dirty range does not map to a segment" }
