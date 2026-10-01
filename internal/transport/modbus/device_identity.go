// internal/transport/modbus/device_identity.go
package modbus

import (
	"fmt"
	"mma2/internal/version"
)

// DeviceIdentity contains immutable Basic identification objects.
// Construct it at startup with NewDeviceIdentity. The zero value uses defaults.
type DeviceIdentity struct{ objects [3]string }

// DefaultDeviceIdentity uses the appliance's compiled identity and release version.
func DefaultDeviceIdentity() DeviceIdentity {
	return DeviceIdentity{objects: [3]string{"github.com/tamzrod", "MMA2", version.Version}}
}

// Values returns a copy of the Basic object values in object ID order.
func (identity DeviceIdentity) Values() [3]string { return identity.objects }

// NewDeviceIdentity validates ASCII objects that each fit an indivisible response.
func NewDeviceIdentity(vendor, product, revision string) (DeviceIdentity, error) {
	values := [3]string{vendor, product, revision}
	names := [3]string{"vendor_name", "product_code", "major_minor_revision"}
	for i, value := range values {
		// 253-byte PDU minus seven header bytes and two object header bytes.
		if len(value) == 0 || len(value) > 244 {
			return DeviceIdentity{}, fmt.Errorf("%s: must contain 1..244 ASCII bytes", names[i])
		}
		for j := 0; j < len(value); j++ {
			if value[j] > 0x7f {
				return DeviceIdentity{}, fmt.Errorf("%s: must contain ASCII bytes", names[i])
			}
		}
	}
	return DeviceIdentity{objects: values}, nil
}

// ReadDeviceIdentification implements Modbus Application Protocol section 6.21.
func (identity DeviceIdentity) ReadDeviceIdentification(payload []byte) []byte {
	if identity == (DeviceIdentity{}) {
		identity = DefaultDeviceIdentity()
	}
	if len(payload) == 0 {
		return BuildExceptionPDU(0x2b, 0x03)
	}
	if payload[0] != 0x0e {
		return BuildExceptionPDU(0x2b, 0x01)
	}
	if len(payload) != 3 {
		return BuildExceptionPDU(0x2b, 0x03)
	}
	code, start := payload[1], int(payload[2])
	if code < 1 || code > 4 {
		return BuildExceptionPDU(0x2b, 0x03)
	}
	if start > 2 {
		if code == 4 {
			return BuildExceptionPDU(0x2b, 0x02)
		}
		start = 0 // Unknown stream object restarts identification.
	}
	end := 3
	if code == 4 {
		end = start + 1
	}
	pdu := []byte{0x2b, 0x0e, code, 0x81, 0, 0, 0}
	for i := start; i < end; i++ {
		value := identity.objects[i]
		if len(pdu)+2+len(value) > 253 {
			pdu[4], pdu[5] = 0xff, byte(i)
			break
		}
		pdu = append(pdu, byte(i), byte(len(value)))
		pdu = append(pdu, value...)
		pdu[6]++
	}
	return pdu
}
