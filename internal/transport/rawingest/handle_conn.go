// internal/transport/rawingest/handle_conn.go
package rawingest

import (
	"io"
	"log"
	"net"
	"time"

	"mma2/internal/memorycore"
	"mma2/internal/notify"
)

// HandleConn handles a single Raw Ingest TCP connection.
// It writes exactly 1 byte per packet:
//   0 = OK
//   1 = REJECTED
//
// Notification is optional.
// It is emitted ONLY after successful write commit.
func HandleConn(conn net.Conn, store *memorycore.Store, notifier *notify.Engine) {
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("rawingest: failed to get local TCP address")
		return
	}
	port := uint16(localAddr.Port)

	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("rawingest: failed to get remote TCP address")
		return
	}
	srcIPStr := remoteAddr.IP.String()

	for {
		pkt, err := DecodeOne(conn, port)
		if err != nil {
			if err != io.EOF {
				_, _ = conn.Write([]byte{RespRejected})
			}
			return
		}

		memID := memorycore.MemoryID{
			Port:   pkt.Port,
			UnitID: pkt.UnitID,
		}

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

		// Successful write → optional notify
		if notifier != nil {
			area, ok := mapMemorycoreAreaToNotify(pkt.Area)
			if ok {
				notifier.OnWrite(notify.Event{
					Port:      pkt.Port,
					UnitID:    pkt.UnitID,
					Area:      area,
					Start:     pkt.Address,
					Count:     pkt.Count,
					Source:    notify.SourceRaw,
					SourceIP:  srcIPStr,
					Timestamp: time.Now(),
				})
			}
		}

		_, _ = conn.Write([]byte{RespOK})
	}
}

func mapMemorycoreAreaToNotify(a memorycore.Area) (notify.AreaType, bool) {
	switch a {
	case memorycore.AreaCoils:
		return notify.AreaCoils, true
	case memorycore.AreaDiscreteInputs:
		return notify.AreaDiscreteInputs, true
	case memorycore.AreaHoldingRegs:
		return notify.AreaHoldingRegisters, true
	case memorycore.AreaInputRegs:
		return notify.AreaInputRegisters, true
	default:
		return 0, false
	}
}
