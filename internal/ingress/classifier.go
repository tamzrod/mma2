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
func Classify(conn net.Conn) (Protocol, *bufio.Reader, error) {
	reader := bufio.NewReader(conn)

	// Peek minimal bytes safely
	peek, err := reader.Peek(2)
	if err != nil {
		return ProtocolUnknown, reader, err
	}

	// Raw Ingest magic (example: 'R','I')
	if peek[0] == 'R' && peek[1] == 'I' {
		return ProtocolRawIngest, reader, nil
	}

	// Modbus TCP: first two bytes are Transaction ID (anything valid)
	return ProtocolModbus, reader, nil
}
