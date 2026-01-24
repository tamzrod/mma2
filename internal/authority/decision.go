// internal/authority/decision.go
package authority

// Decision is the result of evaluating a request against:
// 1) state sealing (LOCKED: Modbus exception Device Busy 0x06)
// 2) per-memory access rules (top-down, first match wins)
type Decision struct {
	Allowed       bool
	ExceptionCode uint8
	Reason        string
}

func Allow(reason string) Decision {
	return Decision{Allowed: true, Reason: reason}
}

func Deny(exceptionCode uint8, reason string) Decision {
	return Decision{Allowed: false, ExceptionCode: exceptionCode, Reason: reason}
}
