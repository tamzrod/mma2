package rbe

import (
	"reflect"
	"testing"

	"mma2/internal/memorycore"
)

type captureSink struct { ids []uint8 }
func (s *captureSink) Publish(id uint8) { s.ids = append(s.ids, id) }

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
	if !reflect.DeepEqual(sink.ids, []uint8{1, 2}) { t.Fatalf("initial ids: %v", sink.ids) }
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
