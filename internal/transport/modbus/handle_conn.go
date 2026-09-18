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
	"mma2/internal/rbe"
)

// HandleConn retains the existing API and legacy behavior.
func HandleConn(conn net.Conn, store *memorycore.Store, auth *authority.Authority, notifier *notify.Engine, ae *accessevents.Engine, debug bool) {
	HandleConnWithRBE(conn, store, auth, notifier, nil, ae, debug)
}

// HandleConnWithRBE preserves the order: state sealing, authority, dispatch.
func HandleConnWithRBE(conn net.Conn, store *memorycore.Store, auth *authority.Authority, notifier *notify.Engine, observer *rbe.Engine, ae *accessevents.Engine, debug bool) {
	defer conn.Close()
	localAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("modbus: failed to get local TCP address")
		return
	}
	port := uint16(localAddr.Port)
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
		mid := memorycore.MemoryID{Port: req.Port, UnitID: uint16(req.UnitID)}

		if pdu := stateSealingExceptionPDU(store, req); pdu != nil {
			frame := BuildResponse(req, pdu)
			_, _ = conn.Write(frame)
			continue
		}
		decision := auth.Evaluate(authority.Request{MemoryID: mid, SourceIP: srcIP, FunctionCode: req.FunctionCode})
		if !decision.Allowed {
			if ae != nil { ae.Record(srcIPStr, req.Port, req.UnitID, req.FunctionCode, false) }
			pdu := BuildExceptionPDU(req.FunctionCode, decision.ExceptionCode)
			frame := BuildResponse(req, pdu)
			_, _ = conn.Write(frame)
			continue
		}
		if ae != nil { ae.Record(srcIPStr, req.Port, req.UnitID, req.FunctionCode, true) }

		pdu := DispatchMemoryWithRBE(store, notifier, observer, srcIPStr, req)
		if pdu == nil { return }
		frame := BuildResponse(req, pdu)
		if _, err := conn.Write(frame); err != nil {
			log.Printf("modbus write error: %v", err)
			return
		}
	}
}

func stateSealingExceptionPDU(store *memorycore.Store, req *Request) []byte {
	if store == nil || req == nil { return nil }
	mid := memorycore.MemoryID{Port: req.Port, UnitID: uint16(req.UnitID)}
	mem, ok := store.Get(mid)
	if !ok { return nil }
	seal := mem.StateSealing()
	if seal == nil { return nil }
	buf := []byte{0}
	if err := mem.ReadBits(seal.Area, seal.Address, 1, buf); err != nil {
		return BuildExceptionPDU(req.FunctionCode, seal.ExceptionCode)
	}
	if buf[0]&0x01 == 0 {
		return BuildExceptionPDU(req.FunctionCode, seal.ExceptionCode)
	}
	return nil
}
