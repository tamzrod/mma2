// internal/authority/sealing.go
package authority

import (
	"sync"
	"sync/atomic"

	"MMA2.0/internal/memorycore"
)

// Sealing is policy state (NOT a memory lock).
// Locked behavior: if sealed, Modbus must return Device Busy (0x06).
type Sealing struct {
	mu    sync.RWMutex
	flags map[memorycore.MemoryID]*atomic.Bool
}

func NewSealing() *Sealing {
	return &Sealing{
		flags: make(map[memorycore.MemoryID]*atomic.Bool),
	}
}

func (s *Sealing) Ensure(mid memorycore.MemoryID, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.flags[mid]
	if !ok {
		b = &atomic.Bool{}
		s.flags[mid] = b
	}
	b.Store(enabled)
}

func (s *Sealing) IsSealed(mid memorycore.MemoryID) bool {
	s.mu.RLock()
	b := s.flags[mid]
	s.mu.RUnlock()

	if b == nil {
		return false
	}
	return b.Load()
}

func (s *Sealing) Seal(mid memorycore.MemoryID)   { s.Ensure(mid, true) }
func (s *Sealing) Unseal(mid memorycore.MemoryID) { s.Ensure(mid, false) }
