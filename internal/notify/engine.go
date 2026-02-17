// internal/notify/engine.go
package notify

// Engine handles rule matching and event dispatch.
type Engine struct {
	registry *Registry
	adapter  Adapter
	ch       chan Event
}

// NewEngine creates a notifier engine.
// buffer controls channel capacity.
// If adapter is nil, events are dropped.
func NewEngine(registry *Registry, adapter Adapter, buffer int) *Engine {
	e := &Engine{
		registry: registry,
		adapter:  adapter,
		ch:       make(chan Event, buffer),
	}
	go e.loop()
	return e
}

// OnWrite must be called ONLY after successful write commit.
// It performs rule matching and emits one event per matching rule.
// Non-blocking by design.
func (e *Engine) OnWrite(evt Event) {
	if e == nil || e.registry == nil {
		return
	}

	rules := e.registry.Rules()
	if len(rules) == 0 {
		return
	}

	for _, r := range rules {
		if r.Port != evt.Port || r.UnitID != evt.UnitID {
			continue
		}
		if r.Area != evt.Area {
			continue
		}
		if !rangesOverlap(r.Start, r.Count, evt.Start, evt.Count) {
			continue
		}

		// Create rule-specific event (enriched with rule name)
		enriched := evt
		enriched.Name = r.Name

		// Non-blocking send
		select {
		case e.ch <- enriched:
		default:
			// Drop if channel full (never block write path)
		}
	}
}

func (e *Engine) loop() {
	for evt := range e.ch {
		if e.adapter != nil {
			e.adapter.Emit(evt)
		}
	}
}

func rangesOverlap(startA, countA, startB, countB uint16) bool {
	endA := uint32(startA) + uint32(countA) - 1
	endB := uint32(startB) + uint32(countB) - 1
	return !(endA < uint32(startB) || endB < uint32(startA))
}