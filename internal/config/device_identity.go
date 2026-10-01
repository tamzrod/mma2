// internal/config/device_identity.go
package config

import (
	"fmt"

	"mma2/internal/memorycore"
	"mma2/internal/version"
)

// DeviceIdentityConfig overrides individual compiled defaults. Pointers distinguish
// omitted fields from explicitly empty values, which are rejected.
type DeviceIdentityConfig struct {
	VendorName         *string `yaml:"vendor_name"`
	ProductCode        *string `yaml:"product_code"`
	MajorMinorRevision *string `yaml:"major_minor_revision"`
}

// DeviceIdentityValues is the transport-neutral resolved startup identity.
type DeviceIdentityValues struct {
	VendorName         string
	ProductCode        string
	MajorMinorRevision string
}

// BuildDeviceIdentities resolves startup metadata separately from memory data.
func BuildDeviceIdentities(cfg *Config) (map[memorycore.MemoryID]DeviceIdentityValues, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if cfg.DeviceIdentity != nil {
		return nil, fmt.Errorf("device_identity is no longer supported at root; move its fields to listeners[].memory[].fc43")
	}
	if len(cfg.Memory.Memories) > 0 {
		return nil, fmt.Errorf("legacy memory.memories is no longer supported; define memory under listeners[].memory[] only")
	}
	out := make(map[memorycore.MemoryID]DeviceIdentityValues)
	for li, listener := range cfg.Ingress {
		if len(listener.Memory) == 0 {
			continue
		}
		port, err := parseListenPort(listener.Listen)
		if err != nil {
			return nil, fmt.Errorf("listeners[%d].listen: %w", li, err)
		}
		for mi, def := range listener.Memory {
			path := fmt.Sprintf("listeners[%d].memory[%d]", li, mi)
			if def.UnitID > 255 {
				return nil, fmt.Errorf("%s.unit_id: must be <= 255", path)
			}
			mid := memorycore.MemoryID{Port: port, UnitID: def.UnitID}
			if _, exists := out[mid]; exists {
				return nil, fmt.Errorf("%s: duplicate FC43 identity for port=%d unit_id=%d", path, port, def.UnitID)
			}
			values, err := resolveDeviceIdentity(def.FC43)
			if err != nil {
				return nil, fmt.Errorf("%s.fc43.%w", path, err)
			}
			out[mid] = values
		}
	}
	return out, nil
}

func resolveDeviceIdentity(overrides *DeviceIdentityConfig) (DeviceIdentityValues, error) {
	values := DeviceIdentityValues{
		VendorName:         "github.com/tamzrod",
		ProductCode:        "MMA2",
		MajorMinorRevision: version.Version,
	}

	if overrides != nil {
		if overrides.VendorName != nil {
			values.VendorName = *overrides.VendorName
		}
		if overrides.ProductCode != nil {
			values.ProductCode = *overrides.ProductCode
		}
		if overrides.MajorMinorRevision != nil {
			values.MajorMinorRevision = *overrides.MajorMinorRevision
		}
	}

	fields := []struct {
		name  string
		value string
	}{
		{"vendor_name", values.VendorName},
		{"product_code", values.ProductCode},
		{"major_minor_revision", values.MajorMinorRevision},
	}
	for _, field := range fields {
		if len(field.value) == 0 || len(field.value) > 244 {
			return DeviceIdentityValues{}, fmt.Errorf("%s: must contain 1..244 ASCII bytes", field.name)
		}
		for i := 0; i < len(field.value); i++ {
			if field.value[i] > 0x7f {
				return DeviceIdentityValues{}, fmt.Errorf("%s: must contain ASCII bytes", field.name)
			}
		}
	}

	return values, nil
}
