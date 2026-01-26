// internal/config/build_memory_store.go
package config

import (
	"fmt"
	"strings"

	"MMA2.0/internal/memorycore"
)

// BuildMemoryStore constructs a memorycore.Store from configuration.
//
// CANONICAL MODEL (LOCKED):
//   - Memory is defined ONLY under listeners[].memory[]
//   - Runtime identity is ALWAYS (Port:uint16, UnitID:uint16)
//
// Legacy cfg.Memory.Memories is NOT supported.
// If present, configuration loading MUST fail.
func BuildMemoryStore(cfg *Config) (*memorycore.Store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// Hard fail if legacy model is present
	if len(cfg.Memory.Memories) > 0 {
		return nil, fmt.Errorf(
			"legacy memory.memories is no longer supported; define memory under listeners[].memory[] only",
		)
	}

	store := memorycore.NewStore()

	// ------------------------------------------------------------
	// NESTED LISTENER MODEL (CANONICAL)
	// ------------------------------------------------------------
	for li, listener := range cfg.Ingress {
		if len(listener.Memory) == 0 {
			continue
		}

		// Reuse canonical helper from build_authority_policies.go
		port, err := parseListenPort(listener.Listen)
		if err != nil {
			return nil, fmt.Errorf(
				"listeners[%d] (%s): invalid listen address %q: %w",
				li,
				listener.ID,
				listener.Listen,
				err,
			)
		}

		for mi, def := range listener.Memory {
			key := fmt.Sprintf(
				"listeners[%d] (%s).memory[%d] (unit_id=%d)",
				li,
				listener.ID,
				mi,
				def.UnitID,
			)

			if err := buildOneMemory(store, port, key, def); err != nil {
				return nil, err
			}
		}
	}

	return store, nil
}

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------

// buildOneMemory builds and registers ONE memory instance.
func buildOneMemory(
	store *memorycore.Store,
	port uint16,
	key string,
	def MemoryDefinition,
) error {

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
		return fmt.Errorf("%s: memory create failed: %w", key, err)
	}

	// --------------------
	// State Sealing (presence = enabled)
	// --------------------
	if def.StateSealing != nil {
		area := strings.ToLower(strings.TrimSpace(def.StateSealing.Area))
		if area != "coil" {
			return fmt.Errorf("%s: state_sealing.area must be 'coil'", key)
		}

		if def.Coils.Count == 0 {
			return fmt.Errorf("%s: state_sealing requires coils to be allocated", key)
		}

		start := def.Coils.Start
		count := def.Coils.Count
		addr := def.StateSealing.Address

		endExclusive := uint32(start) + uint32(count)
		if uint32(addr) < uint32(start) || uint32(addr) >= endExclusive {
			return fmt.Errorf(
				"%s: state_sealing.address (%d) out of bounds for coils [%d..%d)",
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

	id := memorycore.MemoryID{
		Port:   port,
		UnitID: def.UnitID,
	}

	if err := store.Add(id, mem); err != nil {
		return fmt.Errorf(
			"%s (port=%d unit=%d): register failed: %w",
			key,
			id.Port,
			id.UnitID,
			err,
		)
	}

	return nil
}
