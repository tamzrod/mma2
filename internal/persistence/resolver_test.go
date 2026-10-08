package persistence

import (
	"testing"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

func allocations() map[memorycore.MemoryID]config.MemoryAllocation {
	return map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: 502, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaCoils:       {Start: 0, Count: 16},
			memorycore.AreaHoldingRegs: {Start: 0, Count: 32},
			memorycore.AreaInputRegs:   {Start: 0, Count: 32},
		}},
		{Port: 503, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaHoldingRegs: {Start: 10, Count: 5},
		}},
	}
}

func TestResolveSegmentsDisabled(t *testing.T) {
	got, err := ResolveSegments(nil, allocations())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("disabled persistence resolved segments: %+v", got)
	}
}

func TestResolveSegmentsRangesEmptyCoversAllocatedAreas(t *testing.T) {
	plan := &config.ResolvedPersistence{RangesEmpty: true}
	got, err := ResolveSegments(plan, allocations())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := got[memorycore.MemoryID{Port: 502, UnitID: 1}]
	if len(a) != 3 {
		t.Fatalf("expected 3 segments for (502,1), got %+v", a)
	}
	wantAreas := []memorycore.Area{memorycore.AreaCoils, memorycore.AreaHoldingRegs, memorycore.AreaInputRegs}
	for i, seg := range a {
		if seg.Area != wantAreas[i] {
			t.Fatalf("segment %d area = %v, want %v", i, seg.Area, wantAreas[i])
		}
	}

	b := got[memorycore.MemoryID{Port: 503, UnitID: 1}]
	if len(b) != 1 || b[0].Area != memorycore.AreaHoldingRegs || b[0].Start != 10 || b[0].Count != 5 {
		t.Fatalf("unexpected segments for (503,1): %+v", b)
	}
}

func TestResolveSegmentsExplicitSubset(t *testing.T) {
	plan := &config.ResolvedPersistence{
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {
				{Area: memorycore.AreaHoldingRegs, Start: 4, Count: 8},
				{Area: memorycore.AreaCoils, Start: 2, Count: 4},
			},
		},
	}
	got, err := ResolveSegments(plan, allocations())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	segs := got[memorycore.MemoryID{Port: 502, UnitID: 1}]
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %+v", segs)
	}
	// Coils rank before holding registers regardless of input order.
	if segs[0].Area != memorycore.AreaCoils || segs[0].Start != 2 || segs[0].Count != 4 {
		t.Fatalf("first segment wrong: %+v", segs[0])
	}
	if segs[1].Area != memorycore.AreaHoldingRegs || segs[1].Start != 4 || segs[1].Count != 8 {
		t.Fatalf("second segment wrong: %+v", segs[1])
	}
	if _, ok := got[memorycore.MemoryID{Port: 503, UnitID: 1}]; ok {
		t.Fatal("unselected identity appeared in explicit plan")
	}
}

func TestResolveSegmentsRejectsUnallocatedSelection(t *testing.T) {
	plan := &config.ResolvedPersistence{
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {{Area: memorycore.AreaDiscreteInputs, Start: 0, Count: 4}},
		},
	}
	if _, err := ResolveSegments(plan, allocations()); err == nil {
		t.Fatal("unallocated selection accepted")
	}
}

func TestResolveSegmentsRejectsOutOfBoundsSelection(t *testing.T) {
	plan := &config.ResolvedPersistence{
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {{Area: memorycore.AreaHoldingRegs, Start: 28, Count: 8}},
		},
	}
	if _, err := ResolveSegments(plan, allocations()); err == nil {
		t.Fatal("out-of-bounds selection accepted")
	}
}

func TestResolveSegmentsRejectsUnknownIdentity(t *testing.T) {
	plan := &config.ResolvedPersistence{
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 9}: {{Area: memorycore.AreaCoils, Start: 0, Count: 1}},
		},
	}
	if _, err := ResolveSegments(plan, allocations()); err == nil {
		t.Fatal("unknown identity accepted")
	}
}

func TestResolveSegmentsRejectsEmptyRangeEntry(t *testing.T) {
	plan := &config.ResolvedPersistence{
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: 502, UnitID: 1}: {},
		},
	}
	if _, err := ResolveSegments(plan, allocations()); err == nil {
		t.Fatal("empty range entry accepted")
	}
}

func TestSortAndCheckSegmentsRejectsOverlap(t *testing.T) {
	segs := []Segment{
		{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 8},
		{Area: memorycore.AreaHoldingRegs, Start: 4, Count: 8},
	}
	err := sortAndCheckSegments(segs)
	if err == nil {
		t.Fatal("overlapping segments accepted")
	}
}

func TestResolveSegmentsDeterministicOrdering(t *testing.T) {
	plan := &config.ResolvedPersistence{RangesEmpty: true}
	first, err := ResolveSegments(plan, allocations())
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveSegments(plan, allocations())
	if err != nil {
		t.Fatal(err)
	}
	for id, a := range first {
		b := second[id]
		if len(a) != len(b) {
			t.Fatalf("segment count mismatch for %v", id)
		}
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("ordering not deterministic for %v: %+v vs %+v", id, a, b)
			}
		}
	}
}
