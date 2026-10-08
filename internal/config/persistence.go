// internal/config/persistence.go
package config

import (
	"fmt"
	"strings"

	"mma2/internal/memorycore"
)

// PersistenceConfig declares native disk-backed state for authoritative raw
// memory. The block is optional and defaults to disabled. Validation of its
// contents is skipped entirely when the block is absent or disabled so that
// existing configurations are unaffected.
type PersistenceConfig struct {
	Enabled   bool               `yaml:"enabled"`
	Directory string             `yaml:"directory"`
	Ranges    []PersistenceRange `yaml:"ranges"`
}

// PersistenceRange selects the persisted subset for one memory identity.
// Identity is (Port, UnitID); YAML keys are lookup context only.
// A range entry with no area selections is invalid.
type PersistenceRange struct {
	Port           uint16           `yaml:"port"`
	UnitID         uint16           `yaml:"unit_id"`
	Coils          *PersistenceArea `yaml:"coils"`
	DiscreteInputs *PersistenceArea `yaml:"discrete_inputs"`
	HoldingRegs    *PersistenceArea `yaml:"holding_registers"`
	InputRegs      *PersistenceArea `yaml:"input_registers"`
}

// PersistenceArea is an explicit, inclusive 16-bit address selection.
type PersistenceArea struct {
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}

func (c *PersistenceConfig) enabled() bool {
	return c != nil && c.Enabled
}

// ResolvedPersistence is the validated, transport-neutral persistence plan.
// It is produced only when persistence is enabled.
type ResolvedPersistence struct {
	Directory string
	// RangesEmpty reports that no explicit ranges were configured, so every
	// configured area of every configured memory identity is persisted.
	RangesEmpty bool
	Ranges      map[memorycore.MemoryID][]ResolvedPersistenceArea
}

// ResolvedPersistenceArea is one validated area selection for a memory identity.
type ResolvedPersistenceArea struct {
	Area  memorycore.Area
	Start uint16
	Count uint16
}

// ValidatePersistence validates the persistence block when enabled.
// It returns nil when the block is absent or disabled.
func ValidatePersistence(cfg *Config) error {
    _, err := BuildPerMemoryPersistencePlans(cfg)
    return err
}

func validatePersistenceRanges(cfg *Config) error {
	seen := make(map[memIdentity]int)
	for ri, r := range cfg.Persistence.Ranges {
		path := fmt.Sprintf("persistence.ranges[%d]", ri)

		if r.Port == 0 {
			return fmt.Errorf("%s.port must be > 0", path)
		}
		if r.UnitID > 0xFF {
			return fmt.Errorf("%s.unit_id must be <= 255", path)
		}
		if !r.hasAreaSelection() {
			return fmt.Errorf("%s: at least one area selection is required", path)
		}

		id := memIdentity{port: r.Port, unit: r.UnitID}
		if prev, ok := seen[id]; ok {
			return fmt.Errorf(
				"%s: duplicate persistence range for (port=%d unit=%d), already defined at persistence.ranges[%d]",
				path, id.port, id.unit, prev,
			)
		}
		seen[id] = ri

		allocations, ok := allocatedAreas(cfg, id)
		if !ok {
			return fmt.Errorf(
				"%s: no configured memory for (port=%d unit=%d)",
				path, id.port, id.unit,
			)
		}

		if err := validatePersistenceAreaSelection(path, "coils", r.Coils, allocations, memorycore.AreaCoils); err != nil {
			return err
		}
		if err := validatePersistenceAreaSelection(path, "discrete_inputs", r.DiscreteInputs, allocations, memorycore.AreaDiscreteInputs); err != nil {
			return err
		}
		if err := validatePersistenceAreaSelection(path, "holding_registers", r.HoldingRegs, allocations, memorycore.AreaHoldingRegs); err != nil {
			return err
		}
		if err := validatePersistenceAreaSelection(path, "input_registers", r.InputRegs, allocations, memorycore.AreaInputRegs); err != nil {
			return err
		}
	}
	return nil
}

func (r PersistenceRange) hasAreaSelection() bool {
	return r.Coils != nil || r.DiscreteInputs != nil || r.HoldingRegs != nil || r.InputRegs != nil
}

func validatePersistenceAreaSelection(
	path, name string,
	sel *PersistenceArea,
	allocations map[memorycore.Area]Area,
	area memorycore.Area,
) error {
	if sel == nil {
		return nil
	}

	if sel.Count == 0 {
		return fmt.Errorf("%s.%s.count must be > 0", path, name)
	}

	alloc, ok := allocations[area]
	if !ok {
		return fmt.Errorf("%s.%s: area is not allocated for this memory", path, name)
	}

	allocEnd := uint32(alloc.Start) + uint32(alloc.Count) // exclusive
	selStart := uint32(sel.Start)
	selEnd := selStart + uint32(sel.Count) // exclusive

	if selStart < uint32(alloc.Start) || selEnd > allocEnd {
		return fmt.Errorf(
			"%s.%s: range [%d..%d) is not contained in allocated area [%d..%d)",
			path, name, sel.Start, uint16(selEnd), alloc.Start, uint16(allocEnd),
		)
	}

	return nil
}

// allocatedAreas returns the configured area allocations for one identity.
func allocatedAreas(cfg *Config, id memIdentity) (map[memorycore.Area]Area, bool) {
	for _, listener := range cfg.Ingress {
		if len(listener.Memory) == 0 {
			continue
		}
		port, err := parseListenPort(listener.Listen)
		if err != nil {
			continue
		}
		if port != id.port {
			continue
		}
		for _, def := range listener.Memory {
			if def.UnitID != id.unit {
				continue
			}
			return areaAllocations(def), true
		}
	}
	return nil, false
}

// areaAllocations maps a memory definition's non-empty areas to their config Area.
func areaAllocations(def MemoryDefinition) map[memorycore.Area]Area {
	out := make(map[memorycore.Area]Area)
	if def.Coils.Count > 0 {
		out[memorycore.AreaCoils] = def.Coils
	}
	if def.DiscreteInputs.Count > 0 {
		out[memorycore.AreaDiscreteInputs] = def.DiscreteInputs
	}
	if def.HoldingRegs.Count > 0 {
		out[memorycore.AreaHoldingRegs] = def.HoldingRegs
	}
	if def.InputRegs.Count > 0 {
		out[memorycore.AreaInputRegs] = def.InputRegs
	}
	return out
}

// MemoryAllocation is the set of allocated areas for one memory identity.
// Only areas with Count > 0 are present.
type MemoryAllocation struct {
	Areas map[memorycore.Area]Area
}

// BuildMemoryAllocations enumerates the allocated areas of every configured
// memory identity, derived from listener (Port) and memory UnitID.
func BuildMemoryAllocations(cfg *Config) map[memorycore.MemoryID]MemoryAllocation {
	out := make(map[memorycore.MemoryID]MemoryAllocation)
	if cfg == nil {
		return out
	}
	for _, listener := range cfg.Ingress {
		if len(listener.Memory) == 0 {
			continue
		}
		port, err := parseListenPort(listener.Listen)
		if err != nil {
			continue
		}
		for _, def := range listener.Memory {
			id := memorycore.MemoryID{Port: port, UnitID: def.UnitID}
			out[id] = MemoryAllocation{Areas: areaAllocations(def)}
		}
	}
	return out
}

// BuildPersistencePlan validates and resolves the persistence configuration
// into a neutral plan. It returns (nil, nil) when persistence is disabled.
func BuildPersistencePlan(cfg *Config) (*ResolvedPersistence, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if err := ValidatePersistence(cfg); err != nil {
		return nil, err
	}
	if !cfg.Persistence.enabled() {
		return nil, nil
	}

	plan := &ResolvedPersistence{
		Directory:   cfg.Persistence.Directory,
		RangesEmpty: len(cfg.Persistence.Ranges) == 0,
		Ranges:      make(map[memorycore.MemoryID][]ResolvedPersistenceArea),
	}

	for _, r := range cfg.Persistence.Ranges {
		id := memorycore.MemoryID{Port: r.Port, UnitID: r.UnitID}
		appendSel := func(sel *PersistenceArea, area memorycore.Area) {
			if sel == nil {
				return
			}
			plan.Ranges[id] = append(plan.Ranges[id], ResolvedPersistenceArea{
				Area:  area,
				Start: sel.Start,
				Count: sel.Count,
			})
		}
		appendSel(r.Coils, memorycore.AreaCoils)
		appendSel(r.DiscreteInputs, memorycore.AreaDiscreteInputs)
		appendSel(r.HoldingRegs, memorycore.AreaHoldingRegs)
		appendSel(r.InputRegs, memorycore.AreaInputRegs)
	}

	return plan, nil
}

// SnapshotFileName returns the deterministic fixed-layout binary snapshot file
// name for an identity. Distinct (Port, UnitID) pairs always map to distinct
// names. The `.bin` extension reflects the fixed-offset layout chosen for
// targeted word/byte updates (see P04/P05).
func SnapshotFileName(id memorycore.MemoryID) string {
	return fmt.Sprintf("mma2-%d-%d.bin", id.Port, id.UnitID)
}

// SnapshotPath returns the full snapshot path for an identity under directory.
func SnapshotPath(directory string, id memorycore.MemoryID) string {
	return strings.TrimRight(directory, "/") + "/" + SnapshotFileName(id)
}
