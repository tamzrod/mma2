// internal/config/build_memory_store.go
package config

import (
	"fmt"

	"MMA2.0/internal/memorycore"
)

// BuildMemoryStore constructs a memorycore.Store from config.Memory.
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
			return nil, fmt.Errorf("memory[%s]: create failed: %w", key, err)
		}

		id := memorycore.MemoryID{
			Port:   key,           // STRING KEY IS THE ID
			UnitID: def.UnitID,
		}

		if err := store.Add(id, mem); err != nil {
			return nil, fmt.Errorf("memory[%s]: register failed: %w", key, err)
		}
	}

	return store, nil
}
