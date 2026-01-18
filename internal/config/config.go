// internal/config/config.go
package config

// Config is the root configuration for MMA2.
// It describes structure only, not behavior.
type Config struct {
	Ingress   []IngressGate   `yaml:"listeners"`
	Transports Transports    `yaml:"transports"`
	Memory    MemoryConfig   `yaml:"memory"`
}

// --------------------
// Ingress
// --------------------

// IngressGate defines a TCP ingress gate.
// It owns the listener and protocol admission only.
type IngressGate struct {
	ID              string          `yaml:"id"`
	Listen          string          `yaml:"listen"`
	Protocols       IngressProtocols `yaml:"protocols"`
	DiscardUnknown  bool            `yaml:"discard_unknown"`
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

	Coils           Area `yaml:"coils"`
	DiscreteInputs  Area `yaml:"discrete_inputs"`
	HoldingRegs     Area `yaml:"holding_registers"`
	InputRegs       Area `yaml:"input_registers"`
}

type Area struct {
	Start uint16 `yaml:"start"`
	Count uint16 `yaml:"count"`
}
