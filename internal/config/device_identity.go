// internal/config/device_identity.go
package config

import (
	"fmt"

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

// BuildDeviceIdentityValues resolves defaults and validates configuration values
// without constructing a transport-layer Modbus object.
func BuildDeviceIdentityValues(cfg *Config) (DeviceIdentityValues, error) {
	values := DeviceIdentityValues{
		VendorName:         "github.com/tamzrod",
		ProductCode:        "MMA2",
		MajorMinorRevision: version.Version,
	}

	if cfg != nil && cfg.DeviceIdentity != nil {
		if cfg.DeviceIdentity.VendorName != nil {
			values.VendorName = *cfg.DeviceIdentity.VendorName
		}
		if cfg.DeviceIdentity.ProductCode != nil {
			values.ProductCode = *cfg.DeviceIdentity.ProductCode
		}
		if cfg.DeviceIdentity.MajorMinorRevision != nil {
			values.MajorMinorRevision = *cfg.DeviceIdentity.MajorMinorRevision
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
			return DeviceIdentityValues{}, fmt.Errorf("device_identity.%s: must contain 1..244 ASCII bytes", field.name)
		}
		for i := 0; i < len(field.value); i++ {
			if field.value[i] > 0x7f {
				return DeviceIdentityValues{}, fmt.Errorf("device_identity.%s: must contain ASCII bytes", field.name)
			}
		}
	}

	return values, nil
}
