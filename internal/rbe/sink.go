package rbe

import "strconv"

// MultiSink fans out to independent, nonblocking sinks.
type MultiSink struct{ Sinks []Sink }

func (m *MultiSink) Publish(id uint8) {
	if m == nil {
		return
	}
	for _, s := range m.Sinks {
		if s != nil {
			s.Publish(id)
		}
	}
}

// RuleIDString retains decimal formatting for diagnostics without inflating
// the one-byte TCP wire format.
func RuleIDString(id uint8) string { return strconv.FormatUint(uint64(id), 10) }
