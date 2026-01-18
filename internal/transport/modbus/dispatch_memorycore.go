// internal/transport/modbus/dispatch_memorycore.go
package modbus

import (
	"MMA2.0/internal/memorycore"
)

// DispatchMemory routes a Modbus request to memorycore.
// Supported:
//   FC3 - Read Holding Registers
//   FC4 - Read Input Registers
func DispatchMemory(store *memorycore.Store, portKey string, req *Request) []byte {
	switch req.FunctionCode {
	case 3:
		return handleReadRegs(store, portKey, req, memorycore.AreaHoldingRegs)
	case 4:
		return handleReadRegs(store, portKey, req, memorycore.AreaInputRegs)
	default:
		// Illegal Function
		return []byte{req.FunctionCode | 0x80, 0x01}
	}
}

func handleReadRegs(
	store *memorycore.Store,
	portKey string,
	req *Request,
	area memorycore.Area,
) []byte {
	if len(req.Payload) < 4 {
		// Illegal Data Value
		return []byte{req.FunctionCode | 0x80, 0x03}
	}

	address := uint16(req.Payload[0])<<8 | uint16(req.Payload[1])
	count := uint16(req.Payload[2])<<8 | uint16(req.Payload[3])

	memID := memorycore.MemoryID{
		Port:   portKey,
		UnitID: uint16(req.UnitID),
	}

	mem, err := store.MustGet(memID)
	if err != nil {
		// Illegal Data Address
		return []byte{req.FunctionCode | 0x80, 0x02}
	}

	buf := make([]byte, int(count)*2)
	if err := mem.ReadRegs(area, address, count, buf); err != nil {
		// Illegal Data Address
		return []byte{req.FunctionCode | 0x80, 0x02}
	}

	pdu := make([]byte, 2+len(buf))
	pdu[0] = req.FunctionCode
	pdu[1] = byte(len(buf))
	copy(pdu[2:], buf)

	return pdu
}
