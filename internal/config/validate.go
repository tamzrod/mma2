// internal/config/validate.go
package config

import "fmt"

// Validate performs structural validation on the loaded configuration.
// It enforces presence, bounds, and consistency only.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	if len(cfg.Ingress) == 0 {
		return fmt.Errorf("listeners: must define at least one ingress gate")
	}

	if err := validateIngress(cfg.Ingress); err != nil {
		return err
	}

	if err := validateMemory(cfg.Memory); err != nil {
		return err
	}

	return nil
}

// --------------------
// Ingress validation
// --------------------

func validateIngress(gates []IngressGate) error {
	seen := make(map[string]struct{})

	for i, g := range gates {
		if g.ID == "" {
			return fmt.Errorf("listeners[%d]: id is required", i)
		}
		if _, ok := seen[g.ID]; ok {
			return fmt.Errorf("listeners[%d]: duplicate id %q", i, g.ID)
		}
		seen[g.ID] = struct{}{}

		if g.Listen == "" {
			return fmt.Errorf("listeners[%d]: listen is required", i)
		}

		if !g.Protocols.Modbus && !g.Protocols.RawIngest {
			return fmt.Errorf(
				"listeners[%d]: at least one protocol must be enabled (modbus or raw_ingest)",
				i,
			)
		}
	}

	return nil
}

// --------------------
// Memory validation
// --------------------

func validateMemory(mem MemoryConfig) error {
	if len(mem.Memories) == 0 {
		return fmt.Errorf("memory: must define at least one memory")
	}

	for key, def := range mem.Memories {
		if def.Port == 0 {
			return fmt.Errorf("memory[%s]: port must be > 0", key)
		}
		if def.UnitID == 0 {
			return fmt.Errorf("memory[%s]: unit_id must be > 0", key)
		}

		if err := validateArea(key, "coils", def.Coils); err != nil {
			return err
		}
		if err := validateArea(key, "discrete_inputs", def.DiscreteInputs); err != nil {
			return err
		}
		if err := validateArea(key, "holding_registers", def.HoldingRegs); err != nil {
			return err
		}
		if err := validateArea(key, "input_registers", def.InputRegs); err != nil {
			return err
		}
	}

	return nil
}

func validateArea(memKey, name string, a Area) error {
	if a.Count == 0 {
		// zero-sized areas are allowed and treated as disabled
		return nil
	}

	end := uint32(a.Start) + uint32(a.Count)
	if end > 0x10000 {
		return fmt.Errorf(
			"memory[%s].%s: start(%d)+count(%d) exceeds 16-bit address space",
			memKey, name, a.Start, a.Count,
		)
	}

	return nil
}
