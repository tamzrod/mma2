// internal/config/build_memory_store.go
package config

import (
	"fmt"
	"strings"

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

		// --------------------
		// State Sealing (presence = enabled)
		// state_sealing:
		//   area: coil
		//   address: 0
		// --------------------
		if def.StateSealing != nil {
			area := strings.ToLower(strings.TrimSpace(def.StateSealing.Area))
			if area != "coil" {
				return nil, fmt.Errorf("memory[%s]: state_sealing.area must be 'coil'", key)
			}

			// Ensure coils are allocated if state sealing references a coil flag.
			if def.Coils.Count == 0 {
				return nil, fmt.Errorf("memory[%s]: state_sealing requires coils to be allocated", key)
			}

			// Bounds check: address must be within [start, start+count-1]
			start := def.Coils.Start
			count := def.Coils.Count
			addr := def.StateSealing.Address

			// Avoid overflow: compute end as uint32
			endExclusive := uint32(start) + uint32(count)
			if uint32(addr) < uint32(start) || uint32(addr) >= endExclusive {
				return nil, fmt.Errorf(
					"memory[%s]: state_sealing.address (%d) out of bounds for coils [%d..%d)",
					key,
					addr,
					start,
					uint16(endExclusive),
				)
			}

			mem.SetStateSealing(memorycore.StateSealingDef{
				Area:    memorycore.AreaCoils,
				Address: addr,
			})
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
