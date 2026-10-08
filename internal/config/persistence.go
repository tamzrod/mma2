// internal/config/persistence.go
package config

import (
	"fmt"
	"path/filepath"

	"mma2/internal/memorycore"
)

// PersistenceConfig is a rejection marker for the unsupported root YAML key.
// Only listeners[].memory[].persistence may configure persistence.
type PersistenceConfig struct{}

// PersistenceArea selects register addresses or bit addresses.
type PersistenceArea struct {
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}

// ResolvedPersistence is the per-identity storage plan consumed by the engine.
type ResolvedPersistence struct {
	Directory string
	RangesEmpty bool
	Ranges map[memorycore.MemoryID][]ResolvedPersistenceArea
}

type ResolvedPersistenceArea struct {
	Area memorycore.Area
	Start uint16
	Count uint16
}

func ValidatePersistence(cfg *Config) error {
	_, err := BuildPerMemoryPersistencePlans(cfg)
	return err
}

// areaAllocations maps only configured, nonempty areas.
func areaAllocations(def MemoryDefinition) map[memorycore.Area]Area {
	out := make(map[memorycore.Area]Area)
	if def.Coils.Count > 0 {out[memorycore.AreaCoils] = def.Coils}
	if def.DiscreteInputs.Count > 0 {out[memorycore.AreaDiscreteInputs] = def.DiscreteInputs}
	if def.HoldingRegs.Count > 0 {out[memorycore.AreaHoldingRegs] = def.HoldingRegs}
	if def.InputRegs.Count > 0 {out[memorycore.AreaInputRegs] = def.InputRegs}
	return out
}

type MemoryAllocation struct {
	Areas map[memorycore.Area]Area
}

func BuildMemoryAllocations(cfg *Config) map[memorycore.MemoryID]MemoryAllocation {
	out := make(map[memorycore.MemoryID]MemoryAllocation)
	if cfg == nil {return out}
	for _, listener := range cfg.Ingress {
		if len(listener.Memory)==0 {continue}
		port,err:=parseListenPort(listener.Listen)
		if err!=nil {continue}
		for _, def := range listener.Memory {
			id:=memorycore.MemoryID{Port:port,UnitID:def.UnitID}
			out[id]=MemoryAllocation{Areas:areaAllocations(def)}
		}
	}
	return out
}

func SnapshotFileName(id memorycore.MemoryID) string {
	return fmt.Sprintf("mma2-%d-%d.bin",id.Port,id.UnitID)
}

func SnapshotPath(directory string,id memorycore.MemoryID) string {
	return filepath.Join(directory,SnapshotFileName(id))
}
