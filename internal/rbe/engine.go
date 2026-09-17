package rbe

import (
	"errors"
	"fmt"

	"mma2/internal/memorycore"
)

// Sink accepts a one-byte RuleID without performing network I/O in Publish.
type Sink interface {
	Publish(id uint8)
}

// Rule identifies one memory range. IDs are unique across the entire engine
// because the wire contains no port, unit, address, or other metadata.
type Rule struct {
	ID     uint8
	Name   string
	Memory memorycore.MemoryID
	Area   memorycore.Area
	Start  uint16
	Count  uint16
}

// Engine is immutable after construction and safe for concurrent write paths.
// It has no last-value cache; memory itself is the state source.
type Engine struct {
	rules map[memorycore.MemoryID][]Rule
	sink  Sink
}

func NewEngine(rules []Rule, sink Sink) (*Engine, error) {
	if sink == nil {
		return nil, errors.New("rbe: sink is required")
	}
	e := &Engine{rules: make(map[memorycore.MemoryID][]Rule), sink: sink}
	seen := make(map[uint8]struct{})
	for _, r := range rules {
		if r.ID == 0 {
			return nil, errors.New("rbe: rule ID 0 is reserved")
		}
		if _, exists := seen[r.ID]; exists {
			return nil, fmt.Errorf("rbe: duplicate rule ID %d", r.ID)
		}
		if r.Count == 0 || uint32(r.Start)+uint32(r.Count) > 65536 {
			return nil, fmt.Errorf("rbe: rule %d has invalid address range", r.ID)
		}
		if !r.Area.IsBitArea() && !r.Area.IsRegArea() {
			return nil, fmt.Errorf("rbe: rule %d has invalid area", r.ID)
		}
		seen[r.ID] = struct{}{}
		e.rules[r.Memory] = append(e.rules[r.Memory], r)
	}
	return e, nil
}

// WriteBits preserves normal memory write behavior if no rule intersects.
// If observed, previous raw bytes and the pre-write seal bit are captured
// atomically with the commit. Events are emitted only AFTER success.
func (e *Engine) WriteBits(mem *memorycore.Memory, id memorycore.MemoryID, area memorycore.Area, address, count uint16, src []byte) error {
	if e == nil || !e.intersects(id, area, address, count) {
		return mem.WriteBits(area, address, count, src)
	}
	obs, err := mem.WriteBitsObserved(area, address, count, src, sealProbe(mem))
	if err != nil {
		return err
	}
	if sealedBefore(mem, obs) {
		return nil
	}
	e.emitChanged(id, area, address, count, obs.Previous, src)
	return nil
}

// WriteRegs is the register counterpart to WriteBits.
func (e *Engine) WriteRegs(mem *memorycore.Memory, id memorycore.MemoryID, area memorycore.Area, address, count uint16, src []byte) error {
	if e == nil || !e.intersects(id, area, address, count) {
		return mem.WriteRegs(area, address, count, src)
	}
	obs, err := mem.WriteRegsObserved(area, address, count, src, sealProbe(mem))
	if err != nil {
		return err
	}
	if sealedBefore(mem, obs) {
		return nil
	}
	e.emitChanged(id, area, address, count, obs.Previous, src)
	return nil
}

func sealProbe(mem *memorycore.Memory) *memorycore.BitProbe {
	if mem == nil {
		return nil
	}
	def := mem.StateSealing()
	if def == nil {
		return nil
	}
	return &memorycore.BitProbe{Area: def.Area, Address: def.Address}
}

func sealedBefore(mem *memorycore.Memory, obs memorycore.WriteObservation) bool {
	return mem != nil && mem.StateSealing() != nil && !obs.ProbeBefore
}

func (e *Engine) intersects(id memorycore.MemoryID, area memorycore.Area, address, count uint16) bool {
	if e == nil || count == 0 {
		return false
	}
	for _, r := range e.rules[id] {
		if r.Area == area && uint32(address) < uint32(r.Start)+uint32(r.Count) && uint32(r.Start) < uint32(address)+uint32(count) {
			return true
		}
	}
	return false
}

func (e *Engine) emitChanged(id memorycore.MemoryID, area memorycore.Area, address, count uint16, previous, incoming []byte) {
	start := uint32(address)
	end := start + uint32(count)
	for _, rule := range e.rules[id] {
		if rule.Area != area {
			continue
		}
		lo := max(start, uint32(rule.Start))
		hi := min(end, uint32(rule.Start)+uint32(rule.Count))
		if lo >= hi {
			continue
		}
		changed := false
		for addr := lo; addr < hi; addr++ {
			i := int(addr - start)
			if area.IsBitArea() {
				mask := byte(1 << uint(i%8))
				changed = previous[i/8]&mask != incoming[i/8]&mask
			} else {
				changed = previous[i*2] != incoming[i*2] || previous[i*2+1] != incoming[i*2+1]
			}
			if changed {
				break
			}
		}
		if changed {
			e.sink.Publish(rule.ID)
		}
	}
}
