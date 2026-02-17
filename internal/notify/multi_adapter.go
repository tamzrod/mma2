// internal/notify/multi_adapter.go
package notify

// MultiAdapter fan-outs events to multiple adapters.
// It does not alter event semantics.
type MultiAdapter struct {
	adapters []Adapter
}

func NewMultiAdapter(adapters ...Adapter) *MultiAdapter {
	cp := make([]Adapter, len(adapters))
	copy(cp, adapters)
	return &MultiAdapter{adapters: cp}
}

func (m *MultiAdapter) Emit(evt Event) {
	for _, a := range m.adapters {
		if a != nil {
			a.Emit(evt)
		}
	}
}