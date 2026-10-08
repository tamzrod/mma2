// Package persistence resolves and owns native, disk-backed state for MMA2's
// authoritative raw memory. It is not a transport, an RBE subscriber, or a
// State Sealing mechanism, and it never depends on memorycore behavior beyond
// reading and restoring raw values.
package persistence

import (
	"fmt"

	"mma2/internal/config"
	"mma2/internal/memorycore"
)

// Segment is one ordered, contiguous persisted range for a single memory
// identity. Start/Count are address and length in the area's own units
// (bits for bit-areas, registers for register-areas).
type Segment struct {
	Area  memorycore.Area
	Start uint16
	Count uint16
}

// overlapError reports two overlapping segments within the same area.
type overlapError struct {
	Area memorycore.Area
	A, B Segment
}

func (e overlapError) Error() string {
	return fmt.Sprintf(
		"persistence: overlapping %s segments [%d..%d) and [%d..%d)",
		e.Area, e.A.Start, uint16(uint32(e.A.Start)+uint32(e.A.Count)),
		e.B.Start, uint16(uint32(e.B.Start)+uint32(e.B.Count)),
	)
}

// ResolveSegments turns a validated persistence plan into deterministically
// ordered segments per memory identity.
//
//   - When the plan is nil (persistence disabled) it returns nil.
//   - When the plan has no explicit ranges, every configured area of every
//     configured identity is selected in full.
//   - Explicit selections are emitted exactly as validated; unselected areas
//     are absent (their memory still initializes normally).
//
// Within an identity, segments are ordered by area (coils, discrete_inputs,
// holding_registers, input_registers) and then by ascending start address.
// Overlapping segments in the same area are rejected.
func ResolveSegments(plan *config.ResolvedPersistence, allocations map[memorycore.MemoryID]config.MemoryAllocation) (map[memorycore.MemoryID][]Segment, error) {
	if plan == nil {
		return nil, nil
	}

	out := make(map[memorycore.MemoryID][]Segment)

	if plan.RangesEmpty {
		for id, alloc := range allocations {
			segments := fullAreaSegments(alloc)
			if len(segments) > 0 {
				out[id] = segments
			}
		}
		return out, nil
	}

	for id, selections := range plan.Ranges {
		alloc, ok := allocations[id]
		if !ok {
			return nil, fmt.Errorf(
				"persistence: selected identity (port=%d unit=%d) has no configured memory",
				id.Port, id.UnitID,
			)
		}
		segments, err := explicitSegments(id, alloc, selections)
		if err != nil {
			return nil, err
		}
		out[id] = segments
	}

	return out, nil
}

// fullAreaSegments selects every allocated area of one identity in full.
func fullAreaSegments(alloc config.MemoryAllocation) []Segment {
	var segments []Segment
	for _, area := range orderedAreas() {
		a, ok := alloc.Areas[area]
		if !ok || a.Count == 0 {
			continue
		}
		segments = append(segments, Segment{Area: area, Start: a.Start, Count: a.Count})
	}
	return segments
}

// explicitSegments validates and orders the explicit selections of one identity.
func explicitSegments(id memorycore.MemoryID, alloc config.MemoryAllocation, selections []config.ResolvedPersistenceArea) ([]Segment, error) {
	if len(selections) == 0 {
		return nil, fmt.Errorf(
			"persistence: identity (port=%d unit=%d) has an empty range entry",
			id.Port, id.UnitID,
		)
	}

	segments := make([]Segment, 0, len(selections))
	for _, sel := range selections {
		a, ok := alloc.Areas[sel.Area]
		if !ok {
			return nil, fmt.Errorf(
				"persistence: identity (port=%d unit=%d) selection %s is not allocated",
				id.Port, id.UnitID, sel.Area,
			)
		}
		if sel.Count == 0 {
			return nil, fmt.Errorf(
				"persistence: identity (port=%d unit=%d) selection %s has zero count",
				id.Port, id.UnitID, sel.Area,
			)
		}
		selEnd := uint32(sel.Start) + uint32(sel.Count)
		allocEnd := uint32(a.Start) + uint32(a.Count)
		if uint32(sel.Start) < uint32(a.Start) || selEnd > allocEnd {
			return nil, fmt.Errorf(
				"persistence: identity (port=%d unit=%d) selection %s [%d..%d) is not contained in allocated area [%d..%d)",
				id.Port, id.UnitID, sel.Area, sel.Start, uint16(selEnd), a.Start, uint16(allocEnd),
			)
		}
		segments = append(segments, Segment{Area: sel.Area, Start: sel.Start, Count: sel.Count})
	}

	if err := sortAndCheckSegments(segments); err != nil {
		return nil, err
	}
	return segments, nil
}

// orderedAreas returns the fixed area order used for deterministic segments.
func orderedAreas() []memorycore.Area {
	return []memorycore.Area{
		memorycore.AreaCoils,
		memorycore.AreaDiscreteInputs,
		memorycore.AreaHoldingRegs,
		memorycore.AreaInputRegs,
	}
}

// sortAndCheckSegments orders segments by area then start, rejecting overlaps.
func sortAndCheckSegments(segments []Segment) error {
	rank := func(a memorycore.Area) int {
		for i, area := range orderedAreas() {
			if area == a {
				return i
			}
		}
		return len(orderedAreas())
	}

	for i := 1; i < len(segments); i++ {
		for j := i; j > 0; j-- {
			cur, prev := segments[j], segments[j-1]
			if rank(cur.Area) < rank(prev.Area) ||
				(rank(cur.Area) == rank(prev.Area) && cur.Start < prev.Start) {
				segments[j], segments[j-1] = segments[j-1], segments[j]
				continue
			}
			break
		}
	}

	for i := 1; i < len(segments); i++ {
		prev, cur := segments[i-1], segments[i]
		if prev.Area != cur.Area {
			continue
		}
		prevEnd := uint32(prev.Start) + uint32(prev.Count)
		if uint32(cur.Start) < prevEnd {
			return overlapError{Area: cur.Area, A: prev, B: cur}
		}
	}

	return nil
}
