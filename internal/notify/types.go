// internal/notify/types.go
package notify

// AreaType identifies a Modbus memory area for notify rules.
// This is an internal type used by the notification engine.
// It is intentionally transport-agnostic.
type AreaType uint8

const (
	AreaCoils AreaType = iota + 1
	AreaDiscreteInputs
	AreaHoldingRegisters
	AreaInputRegisters
)

// YAMLKey returns the YAML key used in config for this area.
func (a AreaType) YAMLKey() string {
	switch a {
	case AreaCoils:
		return "coils"
	case AreaDiscreteInputs:
		return "discrete_inputs"
	case AreaHoldingRegisters:
		return "holding_registers"
	case AreaInputRegisters:
		return "input_registers"
	default:
		return "unknown"
	}
}

// NotifyRule is the canonical, normalized form of a notify rule.
// It is fully resolved to a memory identity (Port, UnitID).
//
// Rule semantics are locked:
// - Each rule is independent
// - Overlap allowed
// - No merging
// - No deduplication
// - No value inspection
type NotifyRule struct {
	Port   uint16
	UnitID uint16

	Area  AreaType
	Start uint16
	Count uint16

	// Name is optional. If absent in YAML, Name is nil.
	Name *string
}

// Registry is an immutable collection of normalized notify rules.
// Stage 1 builds this registry but does not wire runtime behavior.
type Registry struct {
	rules []NotifyRule
}

// NewRegistry constructs an immutable registry.
// The slice is copied to prevent external mutation.
func NewRegistry(rules []NotifyRule) *Registry {
	cp := make([]NotifyRule, len(rules))
	copy(cp, rules)
	return &Registry{rules: cp}
}

// Rules returns a copy of the registry rules to preserve immutability.
func (r *Registry) Rules() []NotifyRule {
	if r == nil || len(r.rules) == 0 {
		return nil
	}
	out := make([]NotifyRule, len(r.rules))
	copy(out, r.rules)
	return out
}