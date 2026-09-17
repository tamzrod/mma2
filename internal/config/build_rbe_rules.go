package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"mma2/internal/memorycore"
	"mma2/internal/rbe"
)

// BuildRBERules validates the entire RBE configuration before any listeners
// are started and returns immutable, memory-qualified rules. No defaults or
// inferred rule IDs are allowed. Only TCP is implemented in this first slice.
func BuildRBERules(cfg *Config) ([]rbe.Rule, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rbe: config is nil")
	}
	if cfg.RBE == nil {
		for li, listener := range cfg.Ingress {
			for mi, mem := range listener.Memory {
				if mem.RBE != nil {
					return nil, fmt.Errorf("listeners[%d].memory[%d].rbe: global rbe.tcp.listen is required", li, mi)
				}
			}
		}
		return nil, nil
	}
	if cfg.RBE.TCP == nil || strings.TrimSpace(cfg.RBE.TCP.Listen) == "" {
		return nil, fmt.Errorf("rbe.tcp.listen is required")
	}
	_, portText, err := net.SplitHostPort(cfg.RBE.TCP.Listen)
	if err != nil {
		return nil, fmt.Errorf("rbe.tcp.listen: %w", err)
	}
	portNumber, err := strconv.Atoi(portText)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("rbe.tcp.listen: invalid port %q", portText)
	}
	if cfg.Notify != nil {
		return nil, fmt.Errorf("rbe: legacy global notify output cannot coexist with rbe")
	}

	seen := make(map[uint8]string)
	var rules []rbe.Rule
	for li, listener := range cfg.Ingress {
		port, err := parseListenPort(listener.Listen)
		if err != nil {
			return nil, fmt.Errorf("listeners[%d].listen: %w", li, err)
		}
		if int(port) == portNumber {
			return nil, fmt.Errorf("rbe.tcp.listen: port %d conflicts with Modbus/Raw Ingest listener", port)
		}
		for mi, mem := range listener.Memory {
			if mem.RBE == nil {
				continue
			}
			prefix := fmt.Sprintf("listeners[%d].memory[%d].rbe", li, mi)
			if mem.Notify != nil {
				return nil, fmt.Errorf("%s: legacy notify rules cannot coexist with rbe", prefix)
			}
			id := memorycore.MemoryID{Port: port, UnitID: mem.UnitID}
			areas := []struct {
				name string
				area memorycore.Area
				layout Area
				rules []RBERuleConfig
			}{
				{"coils", memorycore.AreaCoils, mem.Coils, mem.RBE.Coils},
				{"discrete_inputs", memorycore.AreaDiscreteInputs, mem.DiscreteInputs, mem.RBE.DiscreteInputs},
				{"holding_registers", memorycore.AreaHoldingRegs, mem.HoldingRegs, mem.RBE.HoldingRegs},
				{"input_registers", memorycore.AreaInputRegs, mem.InputRegs, mem.RBE.InputRegs},
			}
			for _, a := range areas {
				for ri, rule := range a.rules {
					path := fmt.Sprintf("%s.%s[%d]", prefix, a.name, ri)
					if rule.ID == 0 || rule.ID > 255 {
						return nil, fmt.Errorf("%s.id: must be 1..255", path)
					}
					if strings.TrimSpace(rule.Name) == "" {
						return nil, fmt.Errorf("%s.name: must not be empty", path)
					}
					if rule.Count == 0 || uint32(rule.Start)+uint32(rule.Count) > 65536 || a.layout.Count == 0 || uint32(rule.Start) < uint32(a.layout.Start) || uint32(rule.Start)+uint32(rule.Count) > uint32(a.layout.Start)+uint32(a.layout.Count) {
						return nil, fmt.Errorf("%s: rule range must be nonempty and contained in the allocated memory area", path)
					}
					if previous, ok := seen[uint8(rule.ID)]; ok {
						return nil, fmt.Errorf("%s.id: duplicate global ID %d (previously %s)", path, rule.ID, previous)
					}
					seen[uint8(rule.ID)] = path
					rules = append(rules, rbe.Rule{ID: uint8(rule.ID), Name: rule.Name, Memory: id, Area: a.area, Start: rule.Start, Count: rule.Count})
				}
			}
		}
	}
	return rules, nil
}
