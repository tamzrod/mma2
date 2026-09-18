package rbe

import (
	"math/rand"
	"reflect"
	"sync"
	"testing"

	"mma2/internal/memorycore"
)

type captureSink struct {
	mu  sync.Mutex
	ids []uint8
}

func (s *captureSink) Publish(id uint8) {
	s.mu.Lock()
	s.ids = append(s.ids, id)
	s.mu.Unlock()
}

func (s *captureSink) snapshot() []uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]uint8, len(s.ids))
	copy(out, s.ids)
	return out
}

func TestEngineChangedIntersectionAndIdenticalWrites(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		InputRegs: &memorycore.AreaLayout{Start: 0, Size: 20},
	})
	if err != nil { t.Fatal(err) }
	sink := &captureSink{}
	e, err := NewEngine([]Rule{
		{ID: 1, Memory: mid, Area: memorycore.AreaInputRegs, Start: 2, Count: 2},
		{ID: 2, Memory: mid, Area: memorycore.AreaInputRegs, Start: 4, Count: 2},
	}, sink)
	if err != nil { t.Fatal(err) }
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 0, 6, []byte{0, 0, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5}); err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(sink.snapshot(), []uint8{1, 2}) { t.Fatalf("initial ids: %v", sink.snapshot()) }
	// Identical refresh, including an irrelevant high-volume write, emits nothing.
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 0, 6, []byte{0, 0, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5}); err != nil { t.Fatal(err) }
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 8, 1, []byte{0, 9}); err != nil { t.Fatal(err) }
	if len(sink.ids) != 2 { t.Fatalf("identical/unwatched write emitted: %v", sink.ids) }
	// Only the first setpoint is modified.
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 3, 1, []byte{0, 8}); err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(sink.ids, []uint8{1, 2, 1}) { t.Fatalf("changed ids: %v", sink.ids) }
}

func TestEngineSealingPreWriteAndNoCatchup(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils: &memorycore.AreaLayout{Start: 0, Size: 2},
		InputRegs: &memorycore.AreaLayout{Start: 0, Size: 8},
	})
	if err != nil { t.Fatal(err) }
	mem.SetStateSealing(memorycore.StateSealingDef{Area: memorycore.AreaCoils, Address: 0, ExceptionCode: 6})
	sink := &captureSink{}
	e, err := NewEngine([]Rule{
		{ID: 1, Memory: mid, Area: memorycore.AreaInputRegs, Start: 2, Count: 2},
		{ID: 2, Memory: mid, Area: memorycore.AreaCoils, Start: 0, Count: 1},
	}, sink)
	if err != nil { t.Fatal(err) }
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 2, 2, []byte{0, 1, 0, 2}); err != nil { t.Fatal(err) }
	if err := e.WriteBits(mem, mid, memorycore.AreaCoils, 0, 1, []byte{1}); err != nil { t.Fatal(err) }
	if len(sink.ids) != 0 { t.Fatalf("sealed/unseal emitted: %v", sink.ids) }
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 2, 2, []byte{0, 1, 0, 2}); err != nil { t.Fatal(err) }
	if len(sink.ids) != 0 { t.Fatalf("catch-up event emitted: %v", sink.ids) }
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 2, 2, []byte{0, 1, 0, 3}); err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(sink.ids, []uint8{1}) { t.Fatalf("expected only new change: %v", sink.ids) }
}

func TestEngineCoilsIgnoreUnusedHighBits(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{Coils: &memorycore.AreaLayout{Start: 0, Size: 8}})
	if err != nil { t.Fatal(err) }
	sink := &captureSink{}
	e, err := NewEngine([]Rule{{ID: 1, Memory: mid, Area: memorycore.AreaCoils, Start: 0, Count: 1}}, sink)
	if err != nil { t.Fatal(err) }
	if err := e.WriteBits(mem, mid, memorycore.AreaCoils, 0, 1, []byte{1}); err != nil { t.Fatal(err) }
	if err := e.WriteBits(mem, mid, memorycore.AreaCoils, 0, 1, []byte{255}); err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(sink.ids, []uint8{1}) { t.Fatalf("unused high bits emitted: %v", sink.ids) }
}

func TestEngineOverlappingRulesAreIndependent(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	e, err := NewEngine([]Rule{
		{ID: 10, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 0, Count: 4},
		{ID: 11, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 2, Count: 4},
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.WriteRegs(mem, mid, memorycore.AreaHoldingRegs, 3, 1, []byte{0, 9}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.ids, []uint8{10, 11}) {
		t.Fatalf("overlap should emit both rules: %v", sink.ids)
	}
	if err := e.WriteRegs(mem, mid, memorycore.AreaHoldingRegs, 0, 1, []byte{0, 1}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.ids, []uint8{10, 11, 10}) {
		t.Fatalf("non-overlap should emit only rule 10: %v", sink.ids)
	}
}

func TestEngineUnalignedCoilChangeOnly(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils: &memorycore.AreaLayout{Start: 0, Size: 16},
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	e, err := NewEngine([]Rule{{ID: 4, Memory: mid, Area: memorycore.AreaCoils, Start: 3, Count: 2}}, sink)
	if err != nil {
		t.Fatal(err)
	}
	// Write coils 2-5: only coil 3 is in the rule and changes from 0 to 1.
	if err := e.WriteBits(mem, mid, memorycore.AreaCoils, 2, 4, []byte{0x02}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.ids, []uint8{4}) {
		t.Fatalf("unaligned change ids: %v", sink.ids)
	}
	// Identical intersection (coil 3 stays 1, coil 4 stays 0) plus unused high bits.
	if err := e.WriteBits(mem, mid, memorycore.AreaCoils, 2, 4, []byte{0xf2}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.ids, []uint8{4}) {
		t.Fatalf("unused bits outside intersection emitted: %v", sink.ids)
	}
}

func TestEngineRejectsInvalidRules(t *testing.T) {
	sink := &captureSink{}
	for _, rules := range [][]Rule{
		{{ID: 0, Area: memorycore.AreaCoils, Start: 0, Count: 1}},
		{{ID: 1, Area: memorycore.AreaCoils, Start: 0, Count: 1}, {ID: 1, Area: memorycore.AreaCoils, Start: 1, Count: 1}},
		{{ID: 1, Area: memorycore.AreaCoils, Start: 0, Count: 0}},
		{{ID: 1, Area: memorycore.AreaCoils, Start: 65535, Count: 2}},
		{{ID: 1, Area: memorycore.AreaInvalid, Start: 0, Count: 1}},
	} {
		if _, err := NewEngine(rules, sink); err == nil { t.Fatalf("accepted invalid rules: %+v", rules) }
	}
}

func TestEngineConcurrentWritersEmitPerChangedRule(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	e, err := NewEngine([]Rule{
		{ID: 1, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 0, Count: 1},
		{ID: 2, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 1, Count: 1},
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := e.WriteRegs(mem, mid, memorycore.AreaHoldingRegs, 0, 1, []byte{0, 1}); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := e.WriteRegs(mem, mid, memorycore.AreaHoldingRegs, 1, 1, []byte{0, 2}); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	got := sink.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected one event per rule, got %v", got)
	}
	seen := map[uint8]int{}
	for _, id := range got {
		seen[id]++
	}
	if seen[1] != 1 || seen[2] != 1 {
		t.Fatalf("expected ids 1 and 2 once each: %v", got)
	}
}

// Rules are absolute addresses. A memory whose area starts above zero must be
// compared with the rule's absolute range, not a remapped offset.
func TestEngineRuleAgainstNonZeroAreaStart(t *testing.T) {
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		InputRegs: &memorycore.AreaLayout{Start: 100, Size: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	e, err := NewEngine([]Rule{{ID: 7, Memory: mid, Area: memorycore.AreaInputRegs, Start: 102, Count: 2}}, sink)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte{0, 0, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5}
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 100, 6, src); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.snapshot(), []uint8{7}) {
		t.Fatalf("nonzero-start rule not detected: %v", sink.snapshot())
	}
	if err := e.WriteRegs(mem, mid, memorycore.AreaInputRegs, 100, 6, src); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.snapshot(), []uint8{7}) {
		t.Fatalf("identical rewrite emitted: %v", sink.snapshot())
	}
}

// Differential check: the engine's emit decision must match a simple reference
// model over randomized register and coil writes.
func TestEngineDifferentialRandomWrites(t *testing.T) {
	const regSize = 24
	const coilSize = 48
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: regSize},
		Coils:       &memorycore.AreaLayout{Start: 0, Size: coilSize},
	})
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(7))
	type refRule struct {
		id    uint8
		reg   bool
		start int
		count int
	}
	var refRules []refRule
	var engineRules []Rule
	nextID := uint8(1)
	for i := 0; i < 6; i++ {
		isReg := i%2 == 0
		size := regSize
		if !isReg {
			size = coilSize
		}
		start := rng.Intn(size - 1)
		count := 1 + rng.Intn(size-start)
		area := memorycore.AreaHoldingRegs
		if !isReg {
			area = memorycore.AreaCoils
		}
		refRules = append(refRules, refRule{id: nextID, reg: isReg, start: start, count: count})
		engineRules = append(engineRules, Rule{ID: nextID, Memory: mid, Area: area, Start: uint16(start), Count: uint16(count)})
		nextID++
	}
	sink := &captureSink{}
	e, err := NewEngine(engineRules, sink)
	if err != nil {
		t.Fatal(err)
	}

	shadowRegs := make([]uint16, regSize)
	shadowBits := make([]bool, coilSize)

	for iter := 0; iter < 4000; iter++ {
		useRegs := rng.Intn(2) == 0
		var area memorycore.Area
		var address, count uint16
		var src []byte
		if useRegs {
			area = memorycore.AreaHoldingRegs
			count = uint16(1 + rng.Intn(8))
			address = uint16(rng.Intn(regSize - int(count) + 1))
			src = make([]byte, int(count)*2)
			for i := range src {
				src[i] = byte(rng.Intn(256))
			}
		} else {
			area = memorycore.AreaCoils
			count = uint16(1 + rng.Intn(12))
			address = uint16(rng.Intn(coilSize - int(count) + 1))
			src = make([]byte, (int(count)+7)/8)
			for i := range src {
				src[i] = byte(rng.Intn(256))
			}
		}

		want := map[uint8]bool{}
		for _, r := range refRules {
			if r.reg != useRegs {
				continue
			}
			lo := maxInt(int(address), r.start)
			hi := minInt(int(address)+int(count), r.start+r.count)
			changed := false
			for a := lo; a < hi; a++ {
				off := a - int(address)
				if useRegs {
					newV := uint16(src[off*2])<<8 | uint16(src[off*2+1])
					if shadowRegs[a] != newV {
						changed = true
					}
				} else {
					newB := src[off/8]&(1<<uint(off%8)) != 0
					if shadowBits[a] != newB {
						changed = true
					}
				}
			}
			if changed {
				want[r.id] = true
			}
		}

		for a := int(address); a < int(address)+int(count); a++ {
			off := a - int(address)
			if useRegs {
				shadowRegs[a] = uint16(src[off*2])<<8 | uint16(src[off*2+1])
			} else {
				shadowBits[a] = src[off/8]&(1<<uint(off%8)) != 0
			}
		}

		before := len(sink.snapshot())
		if useRegs {
			if err := e.WriteRegs(mem, mid, area, address, count, src); err != nil {
				t.Fatalf("iter %d: %v", iter, err)
			}
		} else {
			if err := e.WriteBits(mem, mid, area, address, count, src); err != nil {
				t.Fatalf("iter %d: %v", iter, err)
			}
		}
		got := sink.snapshot()[before:]
		if len(want) != len(got) {
			t.Fatalf("iter %d area=%v addr=%d count=%d: want %v got %v", iter, area, address, count, want, got)
		}
		counts := map[uint8]int{}
		for _, id := range got {
			counts[id]++
		}
		for id := range want {
			if counts[id] != 1 {
				t.Fatalf("iter %d: rule %d emitted %d times, all %v", iter, id, counts[id], got)
			}
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
