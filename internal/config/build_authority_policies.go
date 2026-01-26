// internal/config/build_authority_policies.go
package config

import (
	"fmt"
	"net"
	"strconv"

	"MMA2.0/internal/authority"
	"MMA2.0/internal/memorycore"
)

// BuildAuthorityPolicies translates config memory-scoped policy into runtime authority policies.
//
// Architectural rule (LOCKED):
//   Policies are keyed by MemoryID = (Port:uint16, UnitID:uint16).
// YAML memory keys are human/debug context only.
func BuildAuthorityPolicies(cfg *Config) (map[memorycore.MemoryID]*authority.MemoryPolicy, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	out := make(map[memorycore.MemoryID]*authority.MemoryPolicy)

	// ---------------------------
	// Legacy model: cfg.Memory.Memories (canonical runtime model)
	// ---------------------------
	for key, def := range cfg.Memory.Memories {
		if def.Policy == nil {
			continue
		}

		mid := memorycore.MemoryID{
			Port:   def.Port,
			UnitID: def.UnitID,
		}

		if _, exists := out[mid]; exists {
			return nil, fmt.Errorf("duplicate policy for memory (port=%d unit_id=%d) from legacy memory[%s]", mid.Port, mid.UnitID, key)
		}

		p, err := buildPolicyFromDef(def, fmt.Sprintf("memory[%s]", key))
		if err != nil {
			return nil, err
		}

		out[mid] = p
	}

	// ---------------------------
	// Nested model: listeners[].memory[]
	// ---------------------------
	for li, ing := range cfg.Ingress {
		if len(ing.Memory) == 0 {
			continue
		}

		port, err := parseListenPort(ing.Listen)
		if err != nil {
			return nil, fmt.Errorf("listeners[%d] (%s) listen=%q: %w", li, ing.ID, ing.Listen, err)
		}

		for mi, def := range ing.Memory {
			if def.Policy == nil {
				continue
			}

			mid := memorycore.MemoryID{
				Port:   port,      // derived from listener.listen (LOCKED)
				UnitID: def.UnitID, // from nested memory def
			}

			if _, exists := out[mid]; exists {
				return nil, fmt.Errorf(
					"duplicate policy for memory (port=%d unit_id=%d): listeners[%d] (%s).memory[%d] conflicts with an existing definition",
					mid.Port, mid.UnitID, li, ing.ID, mi,
				)
			}

			ctx := fmt.Sprintf("listeners[%d] (%s).memory[%d]", li, ing.ID, mi)
			p, err := buildPolicyFromDef(def, ctx)
			if err != nil {
				return nil, err
			}

			out[mid] = p
		}
	}

	return out, nil
}

func buildPolicyFromDef(def MemoryDefinition, ctx string) (*authority.MemoryPolicy, error) {
	if def.Policy == nil {
		return nil, nil
	}

	p := &authority.MemoryPolicy{
		Rules: make([]*authority.Rule, 0, len(def.Policy.Rules)),
	}

	for i, rc := range def.Policy.Rules {
		r, err := authority.NewRule(rc.ID, rc.SourceIP, rc.AllowFC)
		if err != nil {
			return nil, fmt.Errorf("%s.policy.rules[%d] (%s): %w", ctx, i, rc.ID, err)
		}
		p.Rules = append(p.Rules, r)
	}

	return p, nil
}

func parseListenPort(listen string) (uint16, error) {
	// Expect forms like:
	//   ":502"
	//   "0.0.0.0:502"
	//   "127.0.0.1:1502"
	//   "[::]:502"
	_, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return 0, fmt.Errorf("invalid listen address (expected host:port): %w", err)
	}

	n, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port out of range: %d", n)
	}

	return uint16(n), nil
}
