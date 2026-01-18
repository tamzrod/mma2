// internal/transport/raw/ingest_loop.go
// PURPOSE: Raw Ingest v1 execution loop.
// ALLOWED: framing, header parsing, resolving, applying writes
// FORBIDDEN: retries, logging, semantics, transport logic

package raw

import (
	"bufio"
	"net"
)

// Status bytes (locked)
const (
	StatusOK    byte = 0x00
	StatusError byte = 0x01
)

// Run executes the Raw Ingest v1 loop on a single TCP connection.
// It reads frames, applies writes, and replies with a 1-byte status.
func Run(conn net.Conn, resolver MemoryResolver, maxPacketBytes int) error {
	r := bufio.NewReader(conn)

	for {
		// Read one complete frame (header + payload)
		f, err := readFrame(r, maxPacketBytes)
		if err != nil {
			// Malformed frame → reply ERROR and stop
			_, _ = conn.Write([]byte{StatusError})
			return err
		}

		// Resolve target memory by UnitID (locked terminology)
		mem, ok := resolver.ResolveMemoryByID(f.Header.UnitID)
		if !ok {
			_, _ = conn.Write([]byte{StatusError})
			return ErrRejected
		}

		// Apply payload based on Area
		switch f.Header.Area {

		case AreaCoils:
			values, err := alignBits(f.Payload, f.Header.Count)
			if err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}
			if err := mem.WriteCoils(f.Header.Address, values); err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}

		case AreaDiscreteInputs:
			values, err := alignBits(f.Payload, f.Header.Count)
			if err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}
			if err := mem.WriteDiscreteInputs(f.Header.Address, values); err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}

		case AreaHoldingRegs:
			values, err := alignRegs(f.Payload, f.Header.Count)
			if err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}
			if err := mem.WriteHoldingRegisters(f.Header.Address, values); err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}

		case AreaInputRegs:
			values, err := alignRegs(f.Payload, f.Header.Count)
			if err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}
			if err := mem.WriteInputRegisters(f.Header.Address, values); err != nil {
				_, _ = conn.Write([]byte{StatusError})
				return ErrRejected
			}

		default:
			// Should be unreachable due to header validation
			_, _ = conn.Write([]byte{StatusError})
			return ErrRejected
		}

		// Success for this frame
		if _, err := conn.Write([]byte{StatusOK}); err != nil {
			return err
		}
	}
}
