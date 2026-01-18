// internal/ingress/classifier.go
package ingress

import (
	"bufio"
	"net"
)

// Protocol identifies the ingress protocol.
type Protocol uint8

const (
	ProtocolUnknown Protocol = iota
	ProtocolModbus
	ProtocolRawIngest
)

// Classify peeks at the connection stream and determines protocol.
// It must not consume bytes permanently.
// The returned reader MUST be used for subsequent reads.
func Classify(conn net.Conn) (Protocol, *bufio.Reader, error) {
	reader := bufio.NewReader(conn)

	peek, err := reader.Peek(2)
	if err != nil {
		return ProtocolUnknown, reader, err
	}

	// Raw Ingest magic: 'R','I'
	if peek[0] == 'R' && peek[1] == 'I' {
		return ProtocolRawIngest, reader, nil
	}

	// Default to Modbus (MBAP TransactionID is arbitrary)
	return ProtocolModbus, reader, nil
}
