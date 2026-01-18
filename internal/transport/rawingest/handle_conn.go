// internal/transport/rawingest/handle_conn.go
package rawingest

import (
	"io"
	"log"
	"net"

	"MMA2.0/internal/memorycore"
)

// HandleConn handles a single Raw Ingest TCP connection.
// It writes exactly 1 byte per packet:
//   0 = OK
//   1 = REJECTED
func HandleConn(conn net.Conn, store *memorycore.Store) {
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("rawingest: failed to get local TCP address")
		return
	}
	port := uint16(localAddr.Port)

	for {
		pkt, err := DecodeOne(conn, port)
		if err != nil {
			if err != io.EOF {
				_, _ = conn.Write([]byte{RespRejected})
				log.Printf("rawingest decode error: %v", err)
			}
			return
		}

		memID := memorycore.MemoryID{Port: pkt.Port, UnitID: pkt.UnitID}
		mem, err := store.MustGet(memID)
		if err != nil {
			_, _ = conn.Write([]byte{RespRejected})
			continue
		}

		if pkt.Area.IsBitArea() {
			if err := mem.WriteBits(pkt.Area, pkt.Address, pkt.Count, pkt.Payload); err != nil {
				_, _ = conn.Write([]byte{RespRejected})
				continue
			}
		} else if pkt.Area.IsRegArea() {
			if err := mem.WriteRegs(pkt.Area, pkt.Address, pkt.Count, pkt.Payload); err != nil {
				_, _ = conn.Write([]byte{RespRejected})
				continue
			}
		} else {
			_, _ = conn.Write([]byte{RespRejected})
			continue
		}

		_, _ = conn.Write([]byte{RespOK})
	}
}
