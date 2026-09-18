// internal/transport/rawingest/handle_conn.go
package rawingest

import (
	"errors"
	"io"
	"log"
	"net"
	"time"

	"mma2/internal/memorycore"
	"mma2/internal/notify"
	"mma2/internal/rbe"
)

// HandleConn retains the original public entry point for existing callers.
func HandleConn(conn net.Conn, store *memorycore.Store, notifier *notify.Engine) {
	HandleConnWithRBE(conn, store, notifier, nil)
}

// HandleConnWithRBE handles Raw Ingest. RBE observes the committed write,
// including input registers and discrete inputs, without changing the Raw
// Ingest response-code contract. Notification output never affects the ACK.
func HandleConnWithRBE(conn net.Conn, store *memorycore.Store, notifier *notify.Engine, observer *rbe.Engine) {
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
			if err == io.EOF {
				return
			}
			_, _ = conn.Write([]byte{decodeErrCode(err)})
			return
		}

		memID := memorycore.MemoryID{Port: pkt.Port, UnitID: pkt.UnitID}
		mem, err := store.MustGet(memID)
		if err != nil {
			_, _ = conn.Write([]byte{RespMemoryNotFound})
			return
		}

		if pkt.Area.IsBitArea() {
			if observer != nil {
				err = observer.WriteBits(mem, memID, pkt.Area, pkt.Address, pkt.Count, pkt.Payload)
			} else {
				err = mem.WriteBits(pkt.Area, pkt.Address, pkt.Count, pkt.Payload)
			}
		} else if pkt.Area.IsRegArea() {
			if observer != nil {
				err = observer.WriteRegs(mem, memID, pkt.Area, pkt.Address, pkt.Count, pkt.Payload)
			} else {
				err = mem.WriteRegs(pkt.Area, pkt.Address, pkt.Count, pkt.Payload)
			}
		} else {
			_, _ = conn.Write([]byte{RespInternalError})
			return
		}
		if err != nil {
			_, _ = conn.Write([]byte{writeErrCode(err)})
			return
		}

		// Legacy write-only notify, retained only for non-RBE configurations.
		if notifier != nil {
			area, ok := mapMemorycoreAreaToNotify(pkt.Area)
			if ok {
				notifier.OnWrite(notify.Event{
					Port: pkt.Port, UnitID: pkt.UnitID, Area: area,
					Start: pkt.Address, Count: pkt.Count, Source: notify.SourceRaw,
					SourceIP: srcIPStr, Timestamp: time.Now(),
				})
			}
		}
		_, _ = conn.Write([]byte{RespOK})
	}
}

func decodeErrCode(err error) byte {
	switch {
	case errors.Is(err, ErrInvalidMagic):
		return RespInvalidMagic
	case errors.Is(err, ErrInvalidVersion):
		return RespInvalidVersion
	case errors.Is(err, ErrInvalidArea):
		return RespInvalidArea
	case errors.Is(err, ErrInvalidCount):
		return RespInvalidCount
	case errors.Is(err, ErrInvalidLength):
		return RespInvalidLength
	default:
		return RespInternalError
	}
}

func writeErrCode(err error) byte {
	switch {
	case errors.Is(err, memorycore.ErrOutOfBounds), errors.Is(err, memorycore.ErrStartOverflow):
		return RespOutOfBounds
	default:
		return RespInternalError
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
