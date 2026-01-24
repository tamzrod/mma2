// internal/authority/ipmatch.go
package authority

import (
	"fmt"
	"net/netip"
	"strings"
)

// IPMatcher matches a source IP against allow-list items (bare IP or CIDR).
// Bare IPs are normalized to /32 (IPv4) or /128 (IPv6).
type IPMatcher struct {
	prefixes []netip.Prefix
}

func NewIPMatcher(items []string) (*IPMatcher, error) {
	pfxs := make([]netip.Prefix, 0, len(items))

	for _, raw := range items {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}

		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("authority: invalid cidr %q: %w", s, err)
			}
			pfxs = append(pfxs, p.Masked())
			continue
		}

		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("authority: invalid ip %q: %w", s, err)
		}

		bits := 32
		if addr.Is6() {
			bits = 128
		}
		pfxs = append(pfxs, netip.PrefixFrom(addr, bits))
	}

	return &IPMatcher{prefixes: pfxs}, nil
}

func (m *IPMatcher) Match(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, p := range m.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
