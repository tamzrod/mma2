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
)

// HandleConn handles a single Raw Ingest TCP connection.
// It writes exactly 1 byte per packet:
//
//	0x00 = OK
//	0x10 = INVALID_MAGIC
//	0x11 = INVALID_VERSION
//	0x12 = INVALID_AREA
//	0x13 = INVALID_COUNT
//	0x14 = INVALID_LENGTH
//	0x20 = MEMORY_NOT_FOUND
//	0x21 = OUT_OF_BOUNDS
//	0x30 = INTERNAL_ERROR
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
			if err == io.EOF {
				return
			}
			_, _ = conn.Write([]byte{decodeErrCode(err)})
			return
		}

		memID := memorycore.MemoryID{
			Port:   pkt.Port,
			UnitID: pkt.UnitID,
		}

		mem, err := store.MustGet(memID)
		if err != nil {
			_, _ = conn.Write([]byte{RespMemoryNotFound})
			return
		}

		if pkt.Area.IsBitArea() {
			if err := mem.WriteBits(pkt.Area, pkt.Address, pkt.Count, pkt.Payload); err != nil {
				_, _ = conn.Write([]byte{writeErrCode(err)})
				return
			}
		} else if pkt.Area.IsRegArea() {
			if err := mem.WriteRegs(pkt.Area, pkt.Address, pkt.Count, pkt.Payload); err != nil {
				_, _ = conn.Write([]byte{writeErrCode(err)})
				return
			}
		} else {
			_, _ = conn.Write([]byte{RespInternalError})
			return
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
	case errors.Is(err, memorycore.ErrOutOfBounds),
		errors.Is(err, memorycore.ErrStartOverflow):
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
