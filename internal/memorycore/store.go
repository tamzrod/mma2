// internal/memorycore/store.go
package memorycore

import "sync"

type Store struct {
	mu   sync.RWMutex
	data map[MemoryID]*Memory
}

func NewStore() *Store {
	return &Store{
		data: make(map[MemoryID]*Memory),
	}
}

func (s *Store) Add(id MemoryID, mem *Memory) error {
	if s == nil {
		return ErrNilMemory
	}
	if err := id.Validate(); err != nil {
		return err
	}
	if mem == nil {
		return ErrNilMemory
	}

	s.mu.Lock()
	s.data[id] = mem
	s.mu.Unlock()

	return nil
}

func (s *Store) Get(id MemoryID) (*Memory, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	mem, ok := s.data[id]
	s.mu.RUnlock()
	return mem, ok
}

func (s *Store) MustGet(id MemoryID) (*Memory, error) {
	mem, ok := s.Get(id)
	if !ok {
		return nil, ErrUnknownMemoryID
	}
	return mem, nil
}
