package config

import (
	"fmt"
	"net/netip"
	"strings"
)

// Validate performs structural validation on the loaded configuration.
// It enforces bounds and consistency only, and supports BOTH config shapes:
//
//   1) Legacy global memory model: cfg.Memory.Memories (each with explicit port)
//   2) Nested listener model: listeners[].memory[] (port inferred from listeners[].listen)
//
// Runtime memory identity is ALWAYS (Port, UnitID).
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	// Ingress is optional in strict structural sense; validate only if present.
	if err := validateIngress(cfg.Ingress); err != nil {
		return err
	}

	// Validate memory definitions from BOTH sources and enforce identity consistency.
	if err := validateAllMemories(cfg); err != nil {
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
		// ID is treated as schema identity (consistency).
		if g.ID == "" {
			return fmt.Errorf("listeners[%d]: id is required", i)
		}
		if _, ok := seen[g.ID]; ok {
			return fmt.Errorf("listeners[%d]: duplicate id %q", i, g.ID)
		}
		seen[g.ID] = struct{}{}

		if strings.TrimSpace(g.Listen) == "" {
			return fmt.Errorf("listeners[%d]: listen is required", i)
		}

		// If nested memories exist, the port must be parseable
		if len(g.Memory) > 0 {
			if _, err := parseListenPort(g.Listen); err != nil {
				return fmt.Errorf(
					"listeners[%d] (%s): invalid listen %q: %w",
					i, g.ID, g.Listen, err,
				)
			}
		}
	}

	return nil
}

// --------------------
// Memory validation (both models)
// --------------------

type memIdentity struct {
	port uint16
	unit uint16
}

func validateAllMemories(cfg *Config) error {
	seen := make(map[memIdentity]string)

	// 1) Legacy model
	for key, def := range cfg.Memory.Memories {
		if err := validateLegacyMemoryDef(key, def); err != nil {
			return err
		}

		id := memIdentity{port: def.Port, unit: def.UnitID}
		if prev, ok := seen[id]; ok {
			return fmt.Errorf(
				"memory identity conflict: (port=%d unit=%d) defined in %s and memory[%s]",
				id.port, id.unit, prev, key,
			)
		}
		seen[id] = fmt.Sprintf("memory[%s]", key)
	}

	// 2) Nested listener model
	for li, l := range cfg.Ingress {
		if len(l.Memory) == 0 {
			continue
		}

		port, err := parseListenPort(l.Listen)
		if err != nil {
			return fmt.Errorf(
				"listeners[%d] (%s): invalid listen %q: %w",
				li, l.ID, l.Listen, err,
			)
		}

		for mi, def := range l.Memory {
			if err := validateNestedMemoryDef(li, mi, l.ID, port, def); err != nil {
				return err
			}

			id := memIdentity{port: port, unit: def.UnitID}
			path := fmt.Sprintf("listeners[%d](%s).memory[%d]", li, l.ID, mi)

			if prev, ok := seen[id]; ok {
				return fmt.Errorf(
					"memory identity conflict: (port=%d unit=%d) defined in %s and %s",
					id.port, id.unit, prev, path,
				)
			}
			seen[id] = path
		}
	}

	return nil
}

func validateLegacyMemoryDef(memKey string, def MemoryDefinition) error {
	if def.Port == 0 {
		return fmt.Errorf("memory[%s]: port must be > 0", memKey)
	}
	if def.UnitID > 0xFF {
		return fmt.Errorf("memory[%s]: unit_id must be <= 255", memKey)
	}

	if err := validateAreas(memKey, def); err != nil {
		return err
	}
	if err := validateStateSealing(memKey, def); err != nil {
		return err
	}
	if err := validatePolicy(memKey, def.Policy); err != nil {
		return err
	}

	return nil
}

func validateNestedMemoryDef(li, mi int, listenerID string, port uint16, def MemoryDefinition) error {
	memKey := fmt.Sprintf("listeners[%d](%s).memory[%d]", li, listenerID, mi)

	if port == 0 {
		return fmt.Errorf("%s: derived port must be > 0", memKey)
	}
	if def.UnitID > 0xFF {
		return fmt.Errorf("%s: unit_id must be <= 255", memKey)
	}

	if err := validateAreas(memKey, def); err != nil {
		return err
	}
	if err := validateStateSealing(memKey, def); err != nil {
		return err
	}
	if err := validatePolicy(memKey, def.Policy); err != nil {
		return err
	}

	return nil
}

func validateAreas(memKey string, def MemoryDefinition) error {
	if err := validateArea(memKey, "coils", def.Coils); err != nil {
		return err
	}
	if err := validateArea(memKey, "discrete_inputs", def.DiscreteInputs); err != nil {
		return err
	}
	if err := validateArea(memKey, "holding_registers", def.HoldingRegs); err != nil {
		return err
	}
	if err := validateArea(memKey, "input_registers", def.InputRegs); err != nil {
		return err
	}
	return nil
}

func validateArea(memKey, name string, a Area) error {
	if a.Count == 0 {
		return nil
	}

	end := uint32(a.Start) + uint32(a.Count)
	if end > 0x10000 {
		return fmt.Errorf(
			"%s.%s: start(%d)+count(%d) exceeds 16-bit address space",
			memKey, name, a.Start, a.Count,
		)
	}

	return nil
}

// --------------------
// State sealing validation (structural only)
// --------------------

func validateStateSealing(memKey string, def MemoryDefinition) error {
	if def.StateSealing == nil {
		return nil
	}

	area := strings.ToLower(strings.TrimSpace(def.StateSealing.Area))
	if area != "coil" {
		return fmt.Errorf("%s.state_sealing.area must be 'coil'", memKey)
	}

	if def.Coils.Count == 0 {
		return fmt.Errorf("%s.state_sealing requires coils to be allocated", memKey)
	}

	start := def.Coils.Start
	count := def.Coils.Count
	addr := def.StateSealing.Address

	endExclusive := uint32(start) + uint32(count)
	if uint32(addr) < uint32(start) || uint32(addr) >= endExclusive {
		return fmt.Errorf(
			"%s.state_sealing.address (%d) out of bounds for coils [%d..%d)",
			memKey, addr, start, uint16(endExclusive),
		)
	}

	return nil
}

// --------------------
// Policy validation (structural only)
// --------------------

func validatePolicy(memKey string, p *MemoryPolicyConfig) error {
	if p == nil {
		return nil
	}

	for i, r := range p.Rules {
		rulePath := fmt.Sprintf("%s.policy.rules[%d]", memKey, i)

		for j, s := range r.SourceIP {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, err := parseIPOrCIDR(s); err != nil {
				return fmt.Errorf(
					"%s.source_ip[%d]: invalid ip/cidr %q: %v",
					rulePath, j, s, err,
				)
			}
		}

		for j, fc := range r.AllowFC {
			if fc == 0 {
				return fmt.Errorf("%s.allow_fc[%d]: invalid function code 0", rulePath, j)
			}
		}
	}

	return nil
}

func parseIPOrCIDR(s string) (any, error) {
	if pfx, err := netip.ParsePrefix(s); err == nil {
		return pfx, nil
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr, nil
	}
	return nil, fmt.Errorf("not an ip or cidr")
}
