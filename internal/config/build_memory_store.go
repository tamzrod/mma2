// internal/config/build_memory_store.go
package config

import (
	"fmt"

	"MMA2.0/internal/memorycore"
)

// BuildMemoryStore constructs a memorycore.Store from config.Memory.
//
// Architectural rule (LOCKED):
//   Memory identity is ALWAYS (Port:uint16, UnitID:uint16).
// YAML map keys are NEVER used as identity.
func BuildMemoryStore(cfg *Config) (*memorycore.Store, error) {
	store := memorycore.NewStore()

	for key, def := range cfg.Memory.Memories {

		layouts := memorycore.MemoryLayouts{}

		if def.Coils.Count > 0 {
			layouts.Coils = &memorycore.AreaLayout{
				Start: def.Coils.Start,
				Size:  def.Coils.Count,
			}
		}

		if def.DiscreteInputs.Count > 0 {
			layouts.DiscreteInputs = &memorycore.AreaLayout{
				Start: def.DiscreteInputs.Start,
				Size:  def.DiscreteInputs.Count,
			}
		}

		if def.HoldingRegs.Count > 0 {
			layouts.HoldingRegs = &memorycore.AreaLayout{
				Start: def.HoldingRegs.Start,
				Size:  def.HoldingRegs.Count,
			}
		}

		if def.InputRegs.Count > 0 {
			layouts.InputRegs = &memorycore.AreaLayout{
				Start: def.InputRegs.Start,
				Size:  def.InputRegs.Count,
			}
		}

		mem, err := memorycore.NewMemory(layouts)
		if err != nil {
			// key is for human/debug context only
			return nil, fmt.Errorf("memory[%s]: create failed: %w", key, err)
		}

		// Identity is numeric and protocol-aligned.
		id := memorycore.MemoryID{
			Port:   def.Port,
			UnitID: def.UnitID,
		}

		if err := store.Add(id, mem); err != nil {
			return nil, fmt.Errorf(
				"memory[%s] (port=%d unit=%d): register failed: %w",
				key,
				id.Port,
				id.UnitID,
				err,
			)
		}
	}

	return store, nil
}
