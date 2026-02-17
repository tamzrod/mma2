// internal/config/build_notify_registry.go
package config

import (
	"fmt"

	"MMA2.0/internal/notify"
)

// BuildNotifyRegistry normalizes notify rules from configuration into an immutable registry.
//
// Stage 1 constraints:
// - YAML parsing + validation + normalization only
// - No runtime wiring
// - No event emission
// - No write path modification
// - No memorycore imports
//
// Canonical source (per current MMA2.x runtime):
// - Nested model: listeners[].memory[] (port derived from listeners[].listen)
// Legacy cfg.Memory.Memories is not supported for notify.
func BuildNotifyRegistry(cfg *Config) (*notify.Registry, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// Keep behavior consistent with the canonical runtime model:
	// legacy memory.memories is rejected (same as BuildMemoryStore).
	if len(cfg.Memory.Memories) > 0 {
		return nil, fmt.Errorf("legacy memory.memories is not supported for notify; define notify under listeners[].memory[] only")
	}

	var rules []notify.NotifyRule

	for li, l := range cfg.Ingress {
		if len(l.Memory) == 0 {
			continue
		}

		port, err := parseListenPort(l.Listen)
		if err != nil {
			return nil, fmt.Errorf("listeners[%d] (%s): invalid listen %q: %w", li, l.ID, l.Listen, err)
		}

		for mi, def := range l.Memory {
			if def.Notify == nil {
				continue
			}

			ctx := fmt.Sprintf("listeners[%d](%s).memory[%d] (unit_id=%d)", li, l.ID, mi, def.UnitID)

			// coils
			if len(def.Notify.Coils) > 0 {
				in := make([]notify.RangeInput, 0, len(def.Notify.Coils))
				for _, r := range def.Notify.Coils {
					in = append(in, notify.RangeInput{Start: r.Start, Count: r.Count, Name: r.Name})
				}
				rs, err := notify.NormalizeRanges(port, def.UnitID, notify.AreaCoils, in)
				if err != nil {
					return nil, fmt.Errorf("%s.notify.coils: %w", ctx, err)
				}
				rules = append(rules, rs...)
			}

			// discrete_inputs
			if len(def.Notify.DiscreteInputs) > 0 {
				in := make([]notify.RangeInput, 0, len(def.Notify.DiscreteInputs))
				for _, r := range def.Notify.DiscreteInputs {
					in = append(in, notify.RangeInput{Start: r.Start, Count: r.Count, Name: r.Name})
				}
				rs, err := notify.NormalizeRanges(port, def.UnitID, notify.AreaDiscreteInputs, in)
				if err != nil {
					return nil, fmt.Errorf("%s.notify.discrete_inputs: %w", ctx, err)
				}
				rules = append(rules, rs...)
			}

			// holding_registers
			if len(def.Notify.HoldingRegs) > 0 {
				in := make([]notify.RangeInput, 0, len(def.Notify.HoldingRegs))
				for _, r := range def.Notify.HoldingRegs {
					in = append(in, notify.RangeInput{Start: r.Start, Count: r.Count, Name: r.Name})
				}
				rs, err := notify.NormalizeRanges(port, def.UnitID, notify.AreaHoldingRegisters, in)
				if err != nil {
					return nil, fmt.Errorf("%s.notify.holding_registers: %w", ctx, err)
				}
				rules = append(rules, rs...)
			}

			// input_registers
			if len(def.Notify.InputRegs) > 0 {
				in := make([]notify.RangeInput, 0, len(def.Notify.InputRegs))
				for _, r := range def.Notify.InputRegs {
					in = append(in, notify.RangeInput{Start: r.Start, Count: r.Count, Name: r.Name})
				}
				rs, err := notify.NormalizeRanges(port, def.UnitID, notify.AreaInputRegisters, in)
				if err != nil {
					return nil, fmt.Errorf("%s.notify.input_registers: %w", ctx, err)
				}
				rules = append(rules, rs...)
			}
		}
	}

	return notify.NewRegistry(rules), nil
}