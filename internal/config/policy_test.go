// internal/config/policy_test.go
//
// Integration tests for IP and function-code permission enforcement.
//
// These tests load test/policy_test.yaml, build authority policies from it
// via BuildAuthorityPolicies, and verify that Authority.Evaluate returns the
// correct decision for every relevant combination of source IP and FC.
//
// Scenarios covered:
//   1. 0.0.0.0/0 policy — any IPv4, full read+write FCs (unit 1, port 5100)
//   2. 0.0.0.0/0 policy — any IPv4, read-only FCs     (unit 2, port 5100)
//   3. Restricted subnet policy                        (unit 3, port 5100)
//   4. No policy at all → default deny                 (unit 1, port 5101)
package config

import (
	"net/netip"
	"path/filepath"
	"runtime"
	"testing"

	"mma2/internal/authority"
	"mma2/internal/memorycore"
)

// testYAMLPath resolves the path to the test YAML relative to this file.
func testYAMLPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// this file is at internal/config/policy_test.go
	// test/policy_test.yaml is two directories up then into test/
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "test", "policy_test.yaml")
}

// buildAuthority loads the test YAML and returns a ready Authority.
func buildAuthority(t *testing.T) *authority.Authority {
	t.Helper()

	path := testYAMLPath(t)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q): %v", path, err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	policies, err := BuildAuthorityPolicies(cfg)
	if err != nil {
		t.Fatalf("BuildAuthorityPolicies: %v", err)
	}

	auth := authority.New()
	for mid, p := range policies {
		auth.SetMemoryPolicy(mid, p)
	}
	return auth
}

func mustAddr(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		panic("mustAddr: " + err.Error())
	}
	return a
}

func mid(port, unitID uint16) memorycore.MemoryID {
	return memorycore.MemoryID{Port: port, UnitID: unitID}
}

// --------------------------------------------------------------------------
// Scenario 1: port 5100 / unit 1 — 0.0.0.0/0, full read+write
// --------------------------------------------------------------------------

func TestPolicy_AllowAll_ReadWrite_AnyIPv4(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5100, 1)

	readFCs := []uint8{1, 2, 3, 4}
	writeFCs := []uint8{5, 6, 15, 16}
	ips := []string{"10.0.0.1", "192.168.0.1", "172.16.5.200", "1.2.3.4"}

	for _, ip := range ips {
		for _, fc := range append(readFCs, writeFCs...) {
			req := authority.Request{MemoryID: m, SourceIP: mustAddr(ip), FunctionCode: fc}
			d := auth.Evaluate(req)
			if !d.Allowed {
				t.Errorf("ip=%s fc=%d: expected Allow, got Deny (%s)", ip, fc, d.Reason)
			}
		}
	}
}

func TestPolicy_AllowAll_ReadWrite_DisallowedFC(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5100, 1)

	// FC 7, 43 are not in the allow list.
	for _, fc := range []uint8{7, 43} {
		req := authority.Request{MemoryID: m, SourceIP: mustAddr("10.0.0.1"), FunctionCode: fc}
		d := auth.Evaluate(req)
		if d.Allowed {
			t.Errorf("fc=%d: expected Deny, got Allow", fc)
		}
		if d.ExceptionCode != authority.ExceptionIllegalFunction {
			t.Errorf("fc=%d: expected exception 0x01, got 0x%02x", fc, d.ExceptionCode)
		}
	}
}

// --------------------------------------------------------------------------
// Scenario 2: port 5100 / unit 2 — 0.0.0.0/0, read-only (FC 1,2,3,4)
// --------------------------------------------------------------------------

func TestPolicy_AllowAll_ReadOnly_ReadFCsAllowed(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5100, 2)

	for _, fc := range []uint8{1, 2, 3, 4} {
		req := authority.Request{MemoryID: m, SourceIP: mustAddr("10.0.0.1"), FunctionCode: fc}
		d := auth.Evaluate(req)
		if !d.Allowed {
			t.Errorf("fc=%d: expected Allow (read-only unit), got Deny (%s)", fc, d.Reason)
		}
	}
}

func TestPolicy_AllowAll_ReadOnly_WriteFCsDenied(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5100, 2)

	for _, fc := range []uint8{5, 6, 15, 16} {
		req := authority.Request{MemoryID: m, SourceIP: mustAddr("10.0.0.1"), FunctionCode: fc}
		d := auth.Evaluate(req)
		if d.Allowed {
			t.Errorf("fc=%d: expected Deny (write FC on read-only unit), got Allow", fc)
		}
		if d.ExceptionCode != authority.ExceptionIllegalFunction {
			t.Errorf("fc=%d: expected exception 0x01, got 0x%02x", fc, d.ExceptionCode)
		}
	}
}

// --------------------------------------------------------------------------
// Scenario 3: port 5100 / unit 3 — restricted subnet (192.168.1.0/24 + 127.0.0.1)
// --------------------------------------------------------------------------

func TestPolicy_RestrictedSubnet_AllowedIPs(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5100, 3)

	allowedIPs := []string{"192.168.1.1", "192.168.1.254", "127.0.0.1"}
	for _, ip := range allowedIPs {
		req := authority.Request{MemoryID: m, SourceIP: mustAddr(ip), FunctionCode: 3}
		d := auth.Evaluate(req)
		if !d.Allowed {
			t.Errorf("ip=%s: expected Allow (in allowed subnet), got Deny (%s)", ip, d.Reason)
		}
	}
}

func TestPolicy_RestrictedSubnet_DeniedIPs(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5100, 3)

	deniedIPs := []string{"10.0.0.1", "192.168.2.1", "8.8.8.8"}
	for _, ip := range deniedIPs {
		req := authority.Request{MemoryID: m, SourceIP: mustAddr(ip), FunctionCode: 3}
		d := auth.Evaluate(req)
		if d.Allowed {
			t.Errorf("ip=%s: expected Deny (not in allowed subnet), got Allow", ip)
		}
		if d.ExceptionCode != authority.ExceptionIllegalFunction {
			t.Errorf("ip=%s: expected exception 0x01, got 0x%02x", ip, d.ExceptionCode)
		}
	}
}

// --------------------------------------------------------------------------
// Scenario 4: port 5101 / unit 1 — NO policy → default deny
// --------------------------------------------------------------------------

func TestPolicy_NoPolicy_DefaultDeny(t *testing.T) {
	auth := buildAuthority(t)
	m := mid(5101, 1)

	// Any IP, any FC — must always be denied when there is no policy.
	cases := []struct {
		ip string
		fc uint8
	}{
		{"10.0.0.1", 3},
		{"192.168.1.1", 1},
		{"127.0.0.1", 6},
		{"0.0.0.0", 16},
	}

	for _, c := range cases {
		req := authority.Request{MemoryID: m, SourceIP: mustAddr(c.ip), FunctionCode: c.fc}
		d := auth.Evaluate(req)
		if d.Allowed {
			t.Errorf("ip=%s fc=%d: expected default Deny (no policy), got Allow", c.ip, c.fc)
		}
		if d.ExceptionCode != authority.ExceptionIllegalFunction {
			t.Errorf("ip=%s fc=%d: expected exception 0x01, got 0x%02x", c.ip, c.fc, d.ExceptionCode)
		}
	}
}
