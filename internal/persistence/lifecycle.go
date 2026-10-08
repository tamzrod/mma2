package persistence

import (
	"sort"
	"sync"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// State is the internal persistence lifecycle state. It is separate from State
// Sealing and is never exposed as an ordinary memory address or a Modbus coil.
type State uint8

const (
	// StateDisabled means persistence is off; no snapshot is read or written.
	StateDisabled State = iota
	// StateRestoring means restore from disk has not yet completed.
	StateRestoring
	// StateReady means restore completed and writes may be persisted.
	StateReady
	// StateFailed means a disk error left persistence unusable; memory keeps
	// running in volatile mode and the failure is observable.
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateDisabled:
		return "DISABLED"
	case StateRestoring:
		return "RESTORING"
	case StateReady:
		return "READY"
	case StateFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// Diagnostics is an immutable snapshot of the persistence owner's observable
// state. It is read-only and safe to log.
type Diagnostics struct {
	State         State
	Directory     string
	Identities    int
	Segments      int
	RestoreSource string
	LastError     string
	LastErrorAt   time.Time
	LastRestoreAt time.Time
	LastSaveAt    time.Time
}

// Manager is the persistence owner. It holds the resolved, ordered segments per
// memory identity and tracks lifecycle state and diagnostics. It owns no
// memorycore behavior; callers supply or query it explicitly.
type Manager struct {
	mu sync.Mutex

	state    State
	dir      string
	segments map[memorycore.MemoryID][]Segment
	layouts  map[memorycore.MemoryID]*Layout

	lastErr     error
	lastErrAt   time.Time
	lastRestore time.Time
	lastSave    time.Time
	restoreSrc  string

	// dirty holds coalesced file byte ranges per identity awaiting flush.
	dirty    map[memorycore.MemoryID][]DirtyRange
	dirtyGen map[memorycore.MemoryID]uint64

	// memories holds the memory instances for persisted identities, needed by
	// the flush path to re-read current values.
	memories map[memorycore.MemoryID]*memorycore.Memory
	notifier func()
}

// New constructs the persistence owner from a validated plan. A nil plan yields
// a DISABLED manager. A non-nil plan starts in RESTORING: restore must complete
// (or fail) before the manager becomes READY.
func New(plan *config.ResolvedPersistence, allocations map[memorycore.MemoryID]config.MemoryAllocation) (*Manager, error) {
	segments, err := ResolveSegments(plan, allocations)
	if err != nil {
		return nil, err
	}

	m := &Manager{}
	if plan == nil {
		m.state = StateDisabled
		return m, nil
	}

	m.state = StateRestoring
	m.dir = plan.Directory
	m.segments = segments
	m.layouts = make(map[memorycore.MemoryID]*Layout, len(segments))
	for id, segs := range segments {
		if len(segs) == 0 {
			continue
		}
		layout, err := NewLayout(id, segs)
		if err != nil {
			return nil, err
		}
		m.layouts[id] = layout
	}
	m.dirty = make(map[memorycore.MemoryID][]DirtyRange)
	m.dirtyGen = make(map[memorycore.MemoryID]uint64)
	m.memories = make(map[memorycore.MemoryID]*memorycore.Memory)
	return m, nil
}

// memory returns the registered memory for an identity.
func (m *Manager) memory(id memorycore.MemoryID) (*memorycore.Memory, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	mem, ok := m.memories[id]
	return mem, ok
}

// LayoutFor returns the cached fixed layout for an identity, if persisted.
func (m *Manager) LayoutFor(id memorycore.MemoryID) (*Layout, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.layouts[id]
	return l, ok
}

// Enabled reports whether persistence is active (not DISABLED).
func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state != StateDisabled
}

// State returns the current lifecycle state.
func (m *Manager) State() State {
	if m == nil {
		return StateDisabled
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Segments returns the resolved segments for an identity.
func (m *Manager) Segments(id memorycore.MemoryID) ([]Segment, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	segs, ok := m.segments[id]
	return segs, ok
}

// PersistedIdentities returns the identities with resolved segments, in a
// deterministic order (ascending port, then unit id). It returns nil when
// persistence is disabled.
func (m *Manager) PersistedIdentities() []memorycore.MemoryID {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	ids := make([]memorycore.MemoryID, 0, len(m.segments))
	for id := range m.segments {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	sort.Slice(ids, func(i, j int) bool {
		if ids[i].Port != ids[j].Port {
			return ids[i].Port < ids[j].Port
		}
		return ids[i].UnitID < ids[j].UnitID
	})
	return ids
}

// Diagnostics returns a consistent snapshot of the manager's observable state.
func (m *Manager) Diagnostics() Diagnostics {
	if m == nil {
		return Diagnostics{State: StateDisabled}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	d := Diagnostics{
		State:         m.state,
		Directory:     m.dir,
		Identities:    len(m.segments),
		RestoreSource: m.restoreSrc,
		LastErrorAt:   m.lastErrAt,
		LastRestoreAt: m.lastRestore,
		LastSaveAt:    m.lastSave,
	}
	for _, segs := range m.segments {
		d.Segments += len(segs)
	}
	if m.lastErr != nil {
		d.LastError = m.lastErr.Error()
	}
	return d
}

// markRestoring records that a restore attempt has begun.
func (m *Manager) markRestoring() {
	m.mu.Lock()
	m.state = StateRestoring
	m.mu.Unlock()
}

// markRestored records a successful restore and transitions to READY.
func (m *Manager) markRestored(at time.Time) {
	m.mu.Lock()
	m.state = StateReady
	m.lastRestore = at
	m.mu.Unlock()
}

// markRestoredFrom records a successful restore and the source used ("primary",
// "backup", or "initial").
func (m *Manager) markRestoredFrom(source string, at time.Time) {
	m.mu.Lock()
	m.state = StateReady
	m.lastRestore = at
	m.restoreSrc = source
	m.mu.Unlock()
}

// markSaved records a successful save.
func (m *Manager) markSaved(at time.Time) {
	m.mu.Lock()
	m.lastSave = at
	m.mu.Unlock()
}

// markFailed records an observable disk failure and transitions to FAILED.
// The original state is retained only in diagnostics; the manager is unusable
// for further persistence until an explicit recovery path is chosen.
func (m *Manager) markFailed(err error, at time.Time) {
	m.mu.Lock()
	m.state = StateFailed
	m.lastErr = err
	m.lastErrAt = at
	m.mu.Unlock()
}
