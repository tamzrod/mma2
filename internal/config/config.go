// internal/config/config.go
package config

// Config is the root configuration for MMA2.
// It describes structure only, not behavior.
type Config struct {
	Ingress    []IngressGate `yaml:"listeners"`
	Transports Transports    `yaml:"transports"`
	Memory     MemoryConfig  `yaml:"memory"`
}

// --------------------
// Ingress
// --------------------

// IngressGate defines a TCP ingress gate.
// It owns the listener and protocol admission only.
type IngressGate struct {
	ID             string           `yaml:"id"`
	Listen         string           `yaml:"listen"`
	Protocols      IngressProtocols `yaml:"protocols"`
	DiscardUnknown bool             `yaml:"discard_unknown"`
}

type IngressProtocols struct {
	Modbus    bool `yaml:"modbus"`
	RawIngest bool `yaml:"raw_ingest"`
}

// --------------------
// Transports
// --------------------

type Transports struct {
	Modbus    ModbusTransport    `yaml:"modbus"`
	RawIngest RawIngestTransport `yaml:"raw_ingest"`
}

type ModbusTransport struct {
	Enabled bool `yaml:"enabled"`
}

type RawIngestTransport struct {
	Enabled bool `yaml:"enabled"`
}

// --------------------
// Memory
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

	// Optional per-memory authorization policy (Phase 3A.5)
	Policy *MemoryPolicyConfig `yaml:"policy"`
}

type Area struct {
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}

// --------------------
// Policy (Phase 3A.5)
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
