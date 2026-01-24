// internal/config/build_authority_policies.go
package config

import (
	"fmt"

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

	for key, def := range cfg.Memory.Memories {
		if def.Policy == nil {
			continue
		}

		mid := memorycore.MemoryID{
			Port:   def.Port,
			UnitID: def.UnitID,
		}

		p := &authority.MemoryPolicy{
			Rules: make([]*authority.Rule, 0, len(def.Policy.Rules)),
		}

		for i, rc := range def.Policy.Rules {
			r, err := authority.NewRule(rc.ID, rc.SourceIP, rc.AllowFC)
			if err != nil {
				return nil, fmt.Errorf("memory[%s].policy.rules[%d] (%s): %w", key, i, rc.ID, err)
			}
			p.Rules = append(p.Rules, r)
		}

		out[mid] = p
	}

	return out, nil
}
