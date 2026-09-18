// internal/authority/authority.go
package authority

import (
	"net/netip"
	"sync"

	"mma2/internal/memorycore"
)

// Modbus exception codes used by access-control decisions.
const (
	ExceptionIllegalFunction = 0x01
)

// Request is the minimum information needed to decide access.
// No Modbus parsing, IO, or memory operations happen here.
// State sealing is not evaluated here; the Modbus transport reads the
// configured memory bit before calling Evaluate.
type Request struct {
	MemoryID     memorycore.MemoryID
	SourceIP     netip.Addr
	FunctionCode uint8
}

// MemoryPolicy is per-memory authorization configuration.
type MemoryPolicy struct {
	// Rules evaluated top-down; first match wins; default deny.
	Rules []*Rule
}

// Authority evaluates memory-scoped access rules (source IP and function code).
type Authority struct {
	mu       sync.RWMutex
	policies map[memorycore.MemoryID]*MemoryPolicy
}

func New() *Authority {
	return &Authority{
		policies: make(map[memorycore.MemoryID]*MemoryPolicy),
	}
}

// SetMemoryPolicy replaces the policy for a memory.
// Intended for startup config load.
func (a *Authority) SetMemoryPolicy(mid memorycore.MemoryID, p *MemoryPolicy) {
	a.mu.Lock()
	a.policies[mid] = p
	a.mu.Unlock()
}

// Evaluate implements the locked order:
// 1) access rules top-down -> first match wins
// 2) default deny if no match or no policy
//
// Coil state sealing is enforced by the Modbus transport before Evaluate.
func (a *Authority) Evaluate(req Request) Decision {
	a.mu.RLock()
	p := a.policies[req.MemoryID]
	a.mu.RUnlock()

	if p == nil || len(p.Rules) == 0 {
		return Deny(ExceptionIllegalFunction, "no access rules (default deny)")
	}

	for _, r := range p.Rules {
		if r == nil {
			continue
		}

		if !r.Matches(req.SourceIP) {
			continue
		}

		// First match wins.
		if r.AllowsFC(req.FunctionCode) {
			return Allow("matched rule: " + r.ID)
		}

		return Deny(ExceptionIllegalFunction, "rule matched but function code not allowed: "+r.ID)
	}

	return Deny(ExceptionIllegalFunction, "no rule matched (default deny)")
}
