// internal/config/config.go
package config

// Config is the root configuration for MMA2.
// It describes structure only, not behavior.
type Config struct {
	Ingress []IngressGate `yaml:"listeners"`
	Memory  MemoryConfig `yaml:"memory"`
}

// --------------------
// Ingress
// --------------------

// IngressGate defines a TCP ingress gate.
// It owns the listener only.
//
// NOTE:
// Memory is OPTIONAL here for the new nested model.
// The legacy global memory model is still supported.
type IngressGate struct {
	ID     string `yaml:"id"`
	Listen string `yaml:"listen"`

	// Optional nested memory definitions (NEW MODEL)
	Memory []MemoryDefinition `yaml:"memory"`
}

// --------------------
// Memory (LEGACY / CANONICAL RUNTIME MODEL)
// --------------------

// MemoryConfig declares all memory layouts.
// Memory identity is (Port, UnitID).
type MemoryConfig struct {
	Memories map[string]MemoryDefinition `yaml:"memories"`
}

type MemoryDefinition struct {
	Port   uint16 `yaml:"port"`
	UnitID uint16 `yaml:"unit_id"`

	Coils          Area `yaml:"coils"`
	DiscreteInputs Area `yaml:"discrete_inputs"`
	HoldingRegs    Area `yaml:"holding_registers"`
	InputRegs      Area `yaml:"input_registers"`

	// Optional state sealing configuration.
	// Presence = enabled.
	StateSealing *StateSealingConfig `yaml:"state_sealing"`

	// Optional per-memory authorization policy
	Policy *MemoryPolicyConfig `yaml:"policy"`
}

type Area struct {
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}

// --------------------
// State Sealing
// --------------------

// StateSealingConfig defines where the sealing flag lives.
// Semantics:
//   0 = sealed
//   1 = unsealed
type StateSealingConfig struct {
	Area    string `yaml:"area"`    // "coil" (only supported value for now)
	Address uint16 `yaml:"address"`
}

// --------------------
// Policy
// --------------------

// MemoryPolicyConfig declares access rules scoped to a single memory definition.
// Rules are evaluated top-down; first match wins; default deny if none match.
type MemoryPolicyConfig struct {
	Rules []PolicyRuleConfig `yaml:"rules"`
}

type PolicyRuleConfig struct {
	ID string `yaml:"id"`

	// CIDR or bare IP strings. Bare IPs are treated as /32 (IPv4) or /128 (IPv6).
	SourceIP []string `yaml:"source_ip"`

	// Allowed Modbus function codes for this rule.
	AllowFC []uint8 `yaml:"allow_fc"`
}
