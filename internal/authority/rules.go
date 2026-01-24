// internal/authority/rules.go
package authority

import (
	"fmt"
	"net/netip"
)

// Rule is a single access-control rule evaluated within a memory policy.
// v1: match source IP; allow list by Modbus function code.
type Rule struct {
	ID string

	IP *IPMatcher

	AllowFunctionCodes map[uint8]struct{}
}

func NewRule(id string, ipList []string, allowFC []uint8) (*Rule, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: rule id required", ErrInvalidRule)
	}

	m, err := NewIPMatcher(ipList)
	if err != nil {
		return nil, err
	}

	allow := make(map[uint8]struct{}, len(allowFC))
	for _, fc := range allowFC {
		allow[fc] = struct{}{}
	}

	return &Rule{
		ID:                 id,
		IP:                 m,
		AllowFunctionCodes: allow,
	}, nil
}

func (r *Rule) Matches(src netip.Addr) bool {
	if r == nil || r.IP == nil {
		return false
	}
	return r.IP.Match(src)
}

func (r *Rule) AllowsFC(fc uint8) bool {
	if r == nil {
		return false
	}
	_, ok := r.AllowFunctionCodes[fc]
	return ok
}
