package modbus_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mma2/internal/authority"
	"mma2/internal/config"
	"mma2/internal/memorycore"
	"mma2/internal/transport/modbus"
	"mma2/internal/version"
)

// Exercise the YAML -> configuration -> transport identity -> TCP response path.
func TestPerMemoryDeviceIdentityYAMLTCP(t *testing.T) {
	listeners := make([]net.Listener, 2)
	for i := range listeners {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners[i] = ln
		defer ln.Close()
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := fmt.Sprintf(`listeners:
  - id: first
    listen: %q
    memory:
      - unit_id: 1
        holding_registers: {start: 0, count: 1}
        fc43:
          vendor_name: First vendor
          product_code: First product
          major_minor_revision: '1.1'
      - unit_id: 2
        holding_registers: {start: 0, count: 1}
        fc43:
          product_code: Second product
      - unit_id: 3
        holding_registers: {start: 0, count: 1}
  - id: second
    listen: %q
    memory:
      - unit_id: 1
        holding_registers: {start: 0, count: 1}
        fc43:
          vendor_name: Other port vendor
`, listeners[0].Addr().String(), listeners[1].Addr().String())
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	store, err := config.BuildMemoryStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	values, err := config.BuildDeviceIdentities(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configured := make(map[memorycore.MemoryID]modbus.DeviceIdentity)
	auth := authority.New()
	rule, err := authority.NewRule("test", []string{"127.0.0.1"}, []uint8{43})
	if err != nil {
		t.Fatal(err)
	}
	for mid, value := range values {
		identity, err := modbus.NewDeviceIdentity(value.VendorName, value.ProductCode, value.MajorMinorRevision)
		if err != nil {
			t.Fatal(err)
		}
		configured[mid] = identity
		auth.SetMemoryPolicy(mid, &authority.MemoryPolicy{Rules: []*authority.Rule{rule}})
	}
	identities := modbus.NewDeviceIdentities(configured)
	// Mutating the construction map must not change runtime metadata.
	for mid := range configured {
		delete(configured, mid)
	}
	for i, ln := range listeners {
		func() {
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := ln.Accept()
				if err == nil {
					modbus.HandleConnWithIdentities(conn, store, auth, nil, nil, nil, false, identities)
				}
			}()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				conn.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("handler did not stop")
				}
			}()
			if err = conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			expected := [][3]string{{"First vendor", "First product", "1.1"}, {"github.com/tamzrod", "Second product", version.Version}, {"github.com/tamzrod", "MMA2", version.Version}}
			if i == 1 {
				expected = [][3]string{{"Other port vendor", "MMA2", version.Version}}
			}
			for unit, objects := range expected {
				frame := []byte{0x12, 0x34, 0, 0, 0, 5, byte(unit + 1), 43, 14, 1, 0}
				if _, err = conn.Write(frame); err != nil {
					t.Fatal(err)
				}
				header := make([]byte, 7)
				if _, err = io.ReadFull(conn, header); err != nil {
					t.Fatal(err)
				}
				pdu := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
				if _, err = io.ReadFull(conn, pdu); err != nil {
					t.Fatal(err)
				}
				want := []byte{43, 14, 1, 0x81, 0, 0, 3}
				for id, value := range objects {
					want = append(want, byte(id), byte(len(value)))
					want = append(want, value...)
				}
				if !bytes.Equal(header[:4], frame[:4]) || header[6] != byte(unit+1) || !bytes.Equal(pdu, want) {
					t.Fatalf("port %d unit %d: got %x want %x", i, unit+1, pdu, want)
				}
			}
		}()
	}
}
