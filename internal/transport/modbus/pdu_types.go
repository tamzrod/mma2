// internal/transport/modbus/pdu_types.go
package modbus

// ReadRequestPDU represents FC 1,2,3,4
type ReadRequestPDU struct {
	Address  uint16
	Quantity uint16
}

// WriteSinglePDU represents FC 5,6
type WriteSinglePDU struct {
	Address uint16
	Value   uint16
}

// WriteMultiplePDU represents FC 16 (write multiple registers)
type WriteMultiplePDU struct {
	Address  uint16
	Quantity uint16
	Values   []uint16
}

// WriteMultipleBitsPDU represents FC 15 (write multiple coils)
type WriteMultipleBitsPDU struct {
	Address  uint16
	Quantity uint16
	Data     []byte
}
