package memorycore

// CommittedWriteObserver is notified after a successful committed write to a
// Memory, while that memory's write lock is held. It is a neutral memory
// primitive: it knows nothing about persistence, RBE, state sealing, rules, or
// output adapters.
//
// Implementations MUST be fast, MUST NOT block, and MUST NOT call back into the
// same Memory (the lock is already held). They must be safe to call from any
// goroutine that performs a committed write.
type CommittedWriteObserver interface {
	// OnCommittedWrite is called once per successful committed write with the
	// exact mutated area, starting address, and unit count.
	OnCommittedWrite(area Area, address, count uint16)
}

// SetCommittedWriteObserver installs the observer. It is intended to be called
// once during startup, before any concurrent writer runs. Passing nil removes
// the observer.
func (m *Memory) SetCommittedWriteObserver(o CommittedWriteObserver) {
	if m == nil {
		return
	}
	m.observer = o
}

// notifyCommitted reports a successful commit to the observer, if any. Callers
// must hold m.mu.
func (m *Memory) notifyCommitted(area Area, address, count uint16) {
	if m == nil || m.observer == nil {
		return
	}
	m.observer.OnCommittedWrite(area, address, count)
}
