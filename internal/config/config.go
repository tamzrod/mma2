// internal/config/config.go
package config

import "mma2/internal/accessevents"

// Config is the root configuration for MMA2.
type Config struct {
	Ingress      []IngressGate                    `yaml:"listeners"`
	Memory       MemoryConfig                     `yaml:"memory"`
	Notify       *NotifyOutputConfig              `yaml:"notify"` // legacy until RBE cutover
	RBE          *RBEOutputConfig                 `yaml:"rbe"`
	AccessEvents *accessevents.AccessEventsConfig `yaml:"access_events"`
	Debug        bool                             `yaml:"debug"`
}

// Legacy write-only notification output.
// Influx is no longer part of MMA; a leftover influx block is rejected at load.
type NotifyOutputConfig struct {
	Influx *yamlRejectedInflux `yaml:"influx"`
}

// yamlRejectedInflux exists only so leftover YAML keys are detected and rejected.
type yamlRejectedInflux struct{}

// RBEOutputConfig is the global RBE output. TCP is required. Influx is rejected.
type RBEOutputConfig struct {
	TCP    *RBETCPConfig       `yaml:"tcp"`
	Influx *yamlRejectedInflux `yaml:"influx"`
}

type RBETCPConfig struct {
	Listen string `yaml:"listen"`
}

// RBERulesConfig declares independent memory-area-based rules.
// IDs are unique globally for the one-byte TCP wire contract.
type RBERulesConfig struct {
	Coils          []RBERuleConfig `yaml:"coils"`
	DiscreteInputs []RBERuleConfig `yaml:"discrete_inputs"`
	HoldingRegs    []RBERuleConfig `yaml:"holding_registers"`
	InputRegs      []RBERuleConfig `yaml:"input_registers"`
}

type RBERuleConfig struct {
	ID    uint16 `yaml:"id"` // validated in 1..255 before narrowing to uint8
	Name  string `yaml:"name"`
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}

// IngressGate owns a TCP ingress listener.
type IngressGate struct {
	ID     string             `yaml:"id"`
	Listen string             `yaml:"listen"`
	Memory []MemoryDefinition `yaml:"memory"`
}

// Legacy configuration shape, rejected by the canonical runtime.
type MemoryConfig struct {
	Memories map[string]MemoryDefinition `yaml:"memories"`
}

type MemoryDefinition struct {
	Port           uint16              `yaml:"port"`
	UnitID         uint16              `yaml:"unit_id"`
	Coils          Area                `yaml:"coils"`
	DiscreteInputs Area                `yaml:"discrete_inputs"`
	HoldingRegs    Area                `yaml:"holding_registers"`
	InputRegs      Area                `yaml:"input_registers"`
	Notify         *NotifyConfig       `yaml:"notify"` // legacy
	RBE            *RBERulesConfig     `yaml:"rbe"`
	StateSealing   *StateSealingConfig `yaml:"state_sealing"`
	Policy         *MemoryPolicyConfig `yaml:"policy"`
}

type Area struct {
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}

// Legacy notification rules, retained only for old configurations.
type NotifyConfig struct {
	Coils          []NotifyRange `yaml:"coils"`
	DiscreteInputs []NotifyRange `yaml:"discrete_inputs"`
	HoldingRegs    []NotifyRange `yaml:"holding_registers"`
	InputRegs      []NotifyRange `yaml:"input_registers"`
}

type NotifyRange struct {
	Start uint16  `yaml:"start"`
	Count uint16  `yaml:"count"`
	Name  *string `yaml:"name,omitempty"`
}

// State sealing: 0=sealed, 1=unsealed.
type StateSealingConfig struct {
	Enabled   *bool  `yaml:"enabled,omitempty"`
	Area      string `yaml:"area"`
	Address   uint16 `yaml:"address"`
	Exception *uint8 `yaml:"exception,omitempty"`
}

// Authority remains scoped per memory.
type MemoryPolicyConfig struct {
	Rules []PolicyRuleConfig `yaml:"rules"`
}

type PolicyRuleConfig struct {
	ID       string   `yaml:"id"`
	SourceIP []string `yaml:"source_ip"`
	AllowFC  []uint8  `yaml:"allow_fc"`
}
