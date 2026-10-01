package config

import (
	"mma2/internal/version"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceIdentityYAML(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		want       [3]string
	}{
		{"absent", "{}", [3]string{"github.com/tamzrod", "MMA2", version.Version}},
		{"all", "device_identity:\n  vendor_name: Vendor\n  product_code: Product\n  major_minor_revision: '3.1'\n", [3]string{"Vendor", "Product", "3.1"}},
		{"partial", "device_identity:\n  product_code: Custom\n", [3]string{"github.com/tamzrod", "Custom", version.Version}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = Validate(cfg); err != nil {
				t.Fatal(err)
			}
			identity, err := BuildDeviceIdentity(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got := identity.Values(); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestDeviceIdentityStartupValidation(t *testing.T) {
	for _, field := range []string{"vendor_name", "product_code", "major_minor_revision"} {
		for _, value := range []string{"", strings.Repeat("x", 245), "é"} {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("device_identity:\n  "+field+": '"+value+"'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = Validate(cfg); err == nil || !strings.Contains(err.Error(), "device_identity."+field) {
				t.Fatalf("field %s: %v", field, err)
			}
		}
	}
}
