// internal/ingress/listener.go
package ingress

import (
	"log"
	"net"

	"MMA2.0/internal/config"
)

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
	proto, err := Classify(conn)
	if err != nil {
		conn.Close()
		return
	}

	switch proto {
	case ProtocolModbus:
		// Modbus is implicit and always enabled
		onModbus(conn)
		return

	case ProtocolRawIngest:
		if l.cfg.Protocols.RawIngest {
			onRawIngest(conn)
			return
		}
	}

	if l.cfg.DiscardUnknown {
		conn.Close()
	}
}
