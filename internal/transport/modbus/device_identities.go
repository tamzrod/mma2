package modbus

import "mma2/internal/memorycore"

// DeviceIdentities is immutable startup metadata, separate from memorycore data.
type DeviceIdentities struct {
	byMemory map[memorycore.MemoryID]DeviceIdentity
}

// NewDeviceIdentities copies the input so subsequent caller changes cannot affect requests.
func NewDeviceIdentities(configured map[memorycore.MemoryID]DeviceIdentity) *DeviceIdentities {
	identities := &DeviceIdentities{byMemory: make(map[memorycore.MemoryID]DeviceIdentity, len(configured))}
	for mid, identity := range configured {
		identities.byMemory[mid] = identity
	}
	return identities
}

func (identities *DeviceIdentities) lookup(mid memorycore.MemoryID, store *memorycore.Store) (DeviceIdentity, bool) {
	if _, exists := store.Get(mid); !exists {
		return DeviceIdentity{}, false
	}
	// Legacy handler APIs use compiled defaults for each existing logical device.
	if identities == nil {
		return DefaultDeviceIdentity(), true
	}
	identity, exists := identities.byMemory[mid]
	return identity, exists
}
