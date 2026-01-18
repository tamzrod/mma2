// internal/ingress/classifier.go
package ingress

import "net"

// Protocol identifies the ingress protocol.
type Protocol uint8

const (
	ProtocolUnknown Protocol = iota
	ProtocolModbus
	ProtocolRawIngest
)

// Classify determines the protocol for an incoming connection.
//
// IMPORTANT:
// This function MUST NOT read from the connection stream.
// Any pre-read will corrupt stream-based protocols like Modbus TCP.
func Classify(conn net.Conn) (Protocol, error) {
	// MMA2.0 is a Modbus Memory Appliance.
	// Modbus TCP is implicit and always enabled.
	// Raw ingest uses explicit routing and should not rely on peeking here.
	return ProtocolModbus, nil
}
