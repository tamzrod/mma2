// internal/memorycore/state_sealing.go
package memorycore

// StateSealingDef describes where the sealing flag lives.
// Semantics:
//   0 = sealed
//   1 = unsealed
type StateSealingDef struct {
	Area    Area
	Address uint16
}

// SetStateSealing attaches a state sealing definition to this memory.
// Metadata only — no behavior.
func (m *Memory) SetStateSealing(def StateSealingDef) {
	m.stateSealing = &def
}

// StateSealing returns the sealing definition, if present.
func (m *Memory) StateSealing() *StateSealingDef {
	return m.stateSealing
}
