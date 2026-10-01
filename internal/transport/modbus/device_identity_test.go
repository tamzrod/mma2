// internal/transport/modbus/device_identity_test.go
package modbus

import (
	"bytes"
	"mma2/internal/version"
	"strings"
	"testing"
)

func TestDeviceIdentification(t *testing.T) {
	identity := DefaultDeviceIdentity()
	values := []string{"github.com/tamzrod", "MMA2", version.Version}
	for _, code := range []byte{1, 2, 3, 4} {
		for _, start := range []byte{0, 1, 2, 3, 255} {
			got := identity.ReadDeviceIdentification([]byte{14, code, start})
			if code == 4 && start > 2 {
				if !bytes.Equal(got, []byte{0xab, 2}) {
					t.Fatalf("unknown individual: %x", got)
				}
				continue
			}
			first := int(start)
			if first > 2 {
				first = 0
			}
			end := 3
			if code == 4 {
				end = first + 1
			}
			want := []byte{43, 14, code, 0x81, 0, 0, byte(end - first)}
			for i := first; i < end; i++ {
				want = append(want, byte(i), byte(len(values[i])))
				want = append(want, values[i]...)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("code %d start %d: got %x want %x", code, start, got, want)
			}
		}
	}
}

func TestDeviceIdentificationInvalidRequests(t *testing.T) {
	for _, tt := range []struct {
		payload   []byte
		exception byte
	}{
		{nil, 3}, {[]byte{14}, 3}, {[]byte{14, 1}, 3}, {[]byte{14, 1, 0, 0}, 3},
		{[]byte{14, 0, 0}, 3}, {[]byte{14, 5, 0}, 3}, {[]byte{14, 255, 0}, 3},
		{[]byte{13, 1, 0}, 1}, {[]byte{0xff}, 1},
	} {
		got := DefaultDeviceIdentity().ReadDeviceIdentification(tt.payload)
		if !bytes.Equal(got, []byte{0xab, tt.exception}) {
			t.Fatalf("%x: got %x", tt.payload, got)
		}
	}
}

func TestDeviceIdentificationPagination(t *testing.T) {
	identity, err := NewDeviceIdentity(strings.Repeat("V", 244), strings.Repeat("P", 244), strings.Repeat("R", 244))
	if err != nil {
		t.Fatal(err)
	}
	for start := byte(0); start < 3; start++ {
		got := identity.ReadDeviceIdentification([]byte{14, 1, start})
		if len(got) != 253 || got[6] != 1 || got[7] != start || got[8] != 244 {
			t.Fatalf("page %d: %x", start, got)
		}
		more, next := byte(0xff), start+1
		if start == 2 {
			more, next = 0, 0
		}
		if got[4] != more || got[5] != next {
			t.Fatalf("continuation %d: %x", start, got[:7])
		}
	}
	// Exact fit with multiple objects and a one-byte-over boundary.
	for _, n := range []int{239, 240} {
		identity, err := NewDeviceIdentity(strings.Repeat("V", n), "P", "R")
		if err != nil {
			t.Fatal(err)
		}
		got := identity.ReadDeviceIdentification([]byte{14, 1, 0})
		if len(got) > 253 || got[6] != 2 || got[4] != 255 || got[5] != 2 {
			t.Fatalf("boundary %d: %x", n, got[:7])
		}
	}
}

func TestDeviceIdentityValidation(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("x", 245), "é"} {
		for field := 0; field < 3; field++ {
			values := [3]string{"vendor", "product", "revision"}
			values[field] = value
			if _, err := NewDeviceIdentity(values[0], values[1], values[2]); err == nil {
				t.Fatalf("accepted field %d value %q", field, value)
			}
		}
	}
}
