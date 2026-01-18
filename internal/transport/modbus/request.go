// internal/transport/modbus/request.go
package modbus

// Request is a fully parsed Modbus TCP request.
// It is transport-local and protocol-mechanical.
type Request struct {
	// TCP context
	Port uint16

	// MBAP
	TransactionID uint16
	ProtocolID    uint16
	Length        uint16

	// PDU
	UnitID       uint8
	FunctionCode uint8
	Payload      []byte
}
