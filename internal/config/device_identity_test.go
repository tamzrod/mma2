// internal/config/device_identity_test.go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mma2/internal/memorycore"
	"mma2/internal/version"
)

func TestDeviceIdentityYAML(t *testing.T) {
	for _, tt := range []struct {
		name string
		yaml string
		want DeviceIdentityValues
	}{
		{"absent", "", DeviceIdentityValues{"github.com/tamzrod", "MMA2", version.Version}},
		{"all", "        fc43:\n          vendor_name: Vendor\n          product_code: Product\n          major_minor_revision: '3.1'\n", DeviceIdentityValues{"Vendor", "Product", "3.1"}},
		{"partial", "        fc43:\n          product_code: Custom\n", DeviceIdentityValues{"github.com/tamzrod", "Custom", version.Version}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("listeners:\n  - id: test\n    listen: '127.0.0.1:1502'\n    memory:\n      - unit_id: 1\n"+tt.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = Validate(cfg); err != nil {
				t.Fatal(err)
			}
			got, err := BuildDeviceIdentities(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got[memorycore.MemoryID{Port: 1502, UnitID: 1}] != tt.want {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestDeviceIdentityStartupValidation(t *testing.T) {
	for _, field := range []string{"vendor_name", "product_code", "major_minor_revision"} {
		for _, value := range []string{"", strings.Repeat("x", 245), "é"} {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("listeners:\n  - id: test\n    listen: '127.0.0.1:1502'\n    memory:\n      - unit_id: 1\n        fc43:\n          "+field+": '"+value+"'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = Validate(cfg); err == nil || !strings.Contains(err.Error(), "listeners[0].memory[0].fc43."+field) {
				t.Fatalf("field %s: %v", field, err)
			}
		}
	}
}

func TestDeviceIdentityRemovedGlobalAndDuplicate(t *testing.T) {
	vendor := "old vendor"
	if err := Validate(&Config{DeviceIdentity: &DeviceIdentityConfig{VendorName: &vendor}}); err == nil || !strings.Contains(err.Error(), "listeners[].memory[].fc43") {
		t.Fatalf("global identity: %v", err)
	}
	cfg := &Config{Ingress: []IngressGate{{ID: "test", Listen: "127.0.0.1:1502", Memory: []MemoryDefinition{{UnitID: 1}, {UnitID: 1}}}}}
	if _, err := BuildDeviceIdentities(cfg); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	if _, err := BuildDeviceIdentities(nil); err == nil {
		t.Fatal("nil config accepted")
	}
}
