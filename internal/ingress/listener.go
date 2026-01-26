// internal/ingress/listener.go
package ingress

import (
	"bufio"
	"log"
	"net"

	"MMA2.0/internal/config"
)

// bufferedConn ensures all reads flow through a bufio.Reader that already peeked.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// Listener represents a TCP ingress gate.
type Listener struct {
	cfg config.IngressGate
}

// NewListener creates a new ingress listener.
func NewListener(cfg config.IngressGate) *Listener {
	return &Listener{cfg: cfg}
}

// ListenAndServe starts the TCP listener and dispatches connections.
func (l *Listener) ListenAndServe(
	onModbus func(net.Conn),
	onRawIngest func(net.Conn),
) error {
	ln, err := net.Listen("tcp", l.cfg.Listen)
	if err != nil {
		return err
	}

	log.Printf("ingress %s listening on %s", l.cfg.ID, l.cfg.Listen)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		go l.handleConn(conn, onModbus, onRawIngest)
	}
}

func (l *Listener) handleConn(
	conn net.Conn,
	onModbus func(net.Conn),
	onRawIngest func(net.Conn),
) {
	proto, reader, err := Classify(conn)
	if err != nil {
		conn.Close()
		return
	}

	// Important: after Peek(), all subsequent reads must use reader.
	bc := &bufferedConn{Conn: conn, r: reader}

	switch proto {
	case ProtocolModbus:
		// Modbus is always enabled
		onModbus(bc)
		return

	case ProtocolRawIngest:
		// Raw ingest is always enabled
		onRawIngest(bc)
		return

	default:
		// Unknown protocol → close
		conn.Close()
	}
}
