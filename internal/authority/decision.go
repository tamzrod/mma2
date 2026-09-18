// internal/authority/decision.go
package authority

// Decision is the result of evaluating a request against per-memory
// access rules (top-down, first match wins). State sealing is not part
// of this decision; the Modbus transport enforces the memory bit first.
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
