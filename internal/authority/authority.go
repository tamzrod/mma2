// internal/authority/authority.go
package authority

import (
	"net/netip"
	"sync"

	"MMA2.0/internal/memorycore"
)

// Modbus exception codes we use.
// Locked: state sealing MUST be Device Busy (0x06).
const (
	ExceptionIllegalFunction = 0x01
	ExceptionDeviceBusy      = 0x06
)

// Request is the minimum information needed to decide access.
// No Modbus parsing, IO, or memory operations happen here.
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

// Authority evaluates state sealing + memory-scoped access rules.
type Authority struct {
	sealing *Sealing

	mu       sync.RWMutex
	policies map[memorycore.MemoryID]*MemoryPolicy
}

func New() *Authority {
	return &Authority{
		sealing:  NewSealing(),
		policies: make(map[memorycore.MemoryID]*MemoryPolicy),
	}
}

func (a *Authority) Sealing() *Sealing { return a.sealing }

// SetMemoryPolicy replaces the policy for a memory.
// Intended for startup config load.
func (a *Authority) SetMemoryPolicy(mid memorycore.MemoryID, p *MemoryPolicy) {
	a.mu.Lock()
	a.policies[mid] = p
	a.mu.Unlock()
}

// Evaluate implements the locked order:
// 1) state sealing check -> Device Busy (0x06)
// 2) access rules top-down -> first match wins
// 3) default deny if no match or no policy
func (a *Authority) Evaluate(req Request) Decision {
	// Step 1: state sealing
	if a.sealing.IsSealed(req.MemoryID) {
		return Deny(ExceptionDeviceBusy, "state sealing enabled")
	}

	// Step 2: rules
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
