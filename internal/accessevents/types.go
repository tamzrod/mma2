// internal/accessevents/types.go
package accessevents

// Key identifies a unique aggregation bucket.
// All six fields are fixed by the design contract.
type Key struct {
	SrcIP        string
	FunctionCode uint8
	Action       string // "read" or "write"
	Status       string // "allowed" or "denied"
	Port         uint16
	Unit         uint16
}

// Event is one access event ready for emission as NDJSON.
// Count and WindowSec are zero-valued on individual (non-summary) events;
// the omitempty tag ensures they are absent from JSON output in that case.
type Event struct {
	Ts           int64  `json:"ts"`
	Port         uint16 `json:"port"`
	Unit         uint16 `json:"unit"`
	FunctionCode uint8  `json:"function_code"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	SrcIP        string `json:"src_ip"`
	Count        int64  `json:"count,omitempty"`
	WindowSec    int    `json:"window_sec,omitempty"`
}

// windowState holds per-key aggregation state.
type windowState struct {
	windowStart     int64 // Unix nanoseconds of the first event in the current window
	suppressedCount int64 // events received after the first that were not emitted
}
