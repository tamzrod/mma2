// internal/transport/modbus/handle_conn.go
package modbus

import (
	"io"
	"log"
	"net"
	"net/netip"

	"mma2/internal/accessevents"
	"mma2/internal/authority"
	"mma2/internal/memorycore"
	"mma2/internal/notify"
)

// HandleConn handles a single Modbus TCP connection.
// debug controls whether protocol-level read errors are logged.
func HandleConn(
	conn net.Conn,
	store *memorycore.Store,
	auth *authority.Authority,
	notifier *notify.Engine, // OPTIONAL
	ae *accessevents.Engine, // OPTIONAL
	debug bool,
) {
	defer conn.Close()

	// Extract local listening port (authoritative)
	localAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("modbus: failed to get local TCP address")
		return
	}
	port := uint16(localAddr.Port)

	// Extract remote source IP
	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("modbus: failed to get remote TCP address")
		return
	}

	srcIP, err := netip.ParseAddr(remoteAddr.IP.String())
	if err != nil {
		log.Printf("modbus: invalid source IP: %v", err)
		return
	}
	srcIPStr := srcIP.String()

	for {
		req, err := ReadRequest(conn, port)
		if err != nil {
			if err != io.EOF && debug {
				log.Printf("modbus read error: %v", err)
			}
			return
		}

		mid := memorycore.MemoryID{
			Port:   req.Port,
			UnitID: uint16(req.UnitID),
		}

		// --------------------
		// STATE SEALING
		// --------------------
		if pdu := stateSealingExceptionPDU(store, req); pdu != nil {
			frame := BuildResponse(req, pdu)
			_, _ = conn.Write(frame)
			continue
		}

		// --------------------
		// ACCESS CONTROL (unchanged)
		// --------------------
		decision := auth.Evaluate(authority.Request{
			MemoryID:     mid,
			SourceIP:     srcIP,
			FunctionCode: req.FunctionCode,
		})

		if !decision.Allowed {
			if ae != nil {
				ae.Record(srcIPStr, req.Port, req.UnitID, req.FunctionCode, false)
			}
			pdu := BuildExceptionPDU(req.FunctionCode, decision.ExceptionCode)
			frame := BuildResponse(req, pdu)
			_, _ = conn.Write(frame)
			continue
		}

		if ae != nil {
			ae.Record(srcIPStr, req.Port, req.UnitID, req.FunctionCode, true)
		}

		// --------------------
		// DISPATCH
		// --------------------
		pdu := DispatchMemory(store, notifier, srcIPStr, req)
		if pdu == nil {
			return
		}

		frame := BuildResponse(req, pdu)
		if _, err := conn.Write(frame); err != nil {
			log.Printf("modbus write error: %v", err)
			return
		}
	}
}

func stateSealingExceptionPDU(store *memorycore.Store, req *Request) []byte {
	if store == nil || req == nil {
		return nil
	}

	mid := memorycore.MemoryID{
		Port:   req.Port,
		UnitID: uint16(req.UnitID),
	}

	mem, ok := store.Get(mid)
	if !ok {
		return nil
	}

	seal := mem.StateSealing()
	if seal == nil {
		return nil
	}

	buf := []byte{0}
	if err := mem.ReadBits(seal.Area, seal.Address, 1, buf); err != nil {
		return BuildExceptionPDU(req.FunctionCode, seal.ExceptionCode)
	}

	if (buf[0] & 0x01) == 0 {
		return BuildExceptionPDU(req.FunctionCode, seal.ExceptionCode)
	}

	return nil
}
