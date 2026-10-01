package config

import (
	"fmt"
	"mma2/internal/transport/modbus"
)

// DeviceIdentityConfig overrides individual compiled defaults. Pointers distinguish
// omitted fields from explicitly empty values, which are rejected.
type DeviceIdentityConfig struct {
	VendorName         *string `yaml:"vendor_name"`
	ProductCode        *string `yaml:"product_code"`
	MajorMinorRevision *string `yaml:"major_minor_revision"`
}

// BuildDeviceIdentity resolves defaults and validates the final startup identity.
func BuildDeviceIdentity(cfg *Config) (modbus.DeviceIdentity, error) {
	values := modbus.DefaultDeviceIdentity().Values()
	if cfg != nil && cfg.DeviceIdentity != nil {
		overrides := [3]*string{cfg.DeviceIdentity.VendorName, cfg.DeviceIdentity.ProductCode, cfg.DeviceIdentity.MajorMinorRevision}
		for i, value := range overrides {
			if value != nil {
				values[i] = *value
			}
		}
	}
	identity, err := modbus.NewDeviceIdentity(values[0], values[1], values[2])
	if err != nil {
		return identity, fmt.Errorf("device_identity.%w", err)
	}
	return identity, nil
}
