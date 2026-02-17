// internal/notify/event.go
package notify

import "time"

// SourceType identifies the origin of the write event.
type SourceType uint8

const (
	SourceModbus SourceType = iota + 1
	SourceRaw
)

// String returns human-readable source.
func (s SourceType) String() string {
	switch s {
	case SourceModbus:
		return "modbus"
	case SourceRaw:
		return "raw"
	default:
		return "unknown"
	}
}

// Event represents a write notification emitted by the engine.
// It is transport-aware but memorycore-agnostic.
type Event struct {
	Port      uint16
	UnitID    uint16
	Area      AreaType
	Start     uint16
	Count     uint16

	// Name is optional and comes from the matched rule.
	Name *string

	Source    SourceType
	SourceIP  string
	Timestamp time.Time
}
