// internal/transport/modbus/dispatch_memorycore.go
package modbus

import (
	"encoding/binary"
	"time"

	"mma2/internal/memorycore"
	"mma2/internal/notify"
	"mma2/internal/rbe"
)

// DispatchMemory retains the original API for existing callers and tests.
func DispatchMemory(store *memorycore.Store, notifier *notify.Engine, sourceIP string, req *Request) []byte {
	return DispatchMemoryWithRBE(store, notifier, nil, sourceIP, req)
}

// DispatchMemoryWithRBE observes successful writes only; reads and Modbus
// responses retain their original behavior. RBE sends no register values.
func DispatchMemoryWithRBE(store *memorycore.Store, notifier *notify.Engine, observer *rbe.Engine, sourceIP string, req *Request) []byte {
	switch req.FunctionCode {
	case 1:
		return handleReadBits(store, req, memorycore.AreaCoils)
	case 2:
		return handleReadBits(store, req, memorycore.AreaDiscreteInputs)
	case 3:
		return handleReadRegs(store, req, memorycore.AreaHoldingRegs)
	case 4:
		return handleReadRegs(store, req, memorycore.AreaInputRegs)
	case 5:
		return handleWriteSingleCoil(store, notifier, observer, sourceIP, req)
	case 6:
		return handleWriteSingleReg(store, notifier, observer, sourceIP, req)
	case 15:
		return handleWriteMultipleCoils(store, notifier, observer, sourceIP, req)
	case 16:
		return handleWriteMultipleRegs(store, notifier, observer, sourceIP, req)
	default:
		return BuildExceptionPDU(req.FunctionCode, 0x01)
	}
}

func resolveMemory(store *memorycore.Store, req *Request) (*memorycore.Memory, bool) {
	mid := memorycore.MemoryID{Port: req.Port, UnitID: uint16(req.UnitID)}
	mem, err := store.MustGet(mid)
	return mem, err == nil
}

func bytesForBits(n uint16) int {
	if n == 0 { return 0 }
	return int((n + 7) / 8)
}

func handleReadBits(store *memorycore.Store, req *Request, area memorycore.Area) []byte {
	decoded, err := DecodeReadRequest(req.Payload)
	if err != nil || decoded.Quantity == 0 {
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}
	mem, ok := resolveMemory(store, req)
	if !ok { return BuildExceptionPDU(req.FunctionCode, 0x02) }
	buf := make([]byte, bytesForBits(decoded.Quantity))
	if err := mem.ReadBits(area, decoded.Address, decoded.Quantity, buf); err != nil {
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}
	return BuildReadResponsePDU(req.FunctionCode, buf)
}

func writeBits(mem *memorycore.Memory, observer *rbe.Engine, req *Request, area memorycore.Area, address, count uint16, src []byte) error {
	if observer == nil { return mem.WriteBits(area, address, count, src) }
	return observer.WriteBits(mem, memorycore.MemoryID{Port: req.Port, UnitID: uint16(req.UnitID)}, area, address, count, src)
}

func writeRegs(mem *memorycore.Memory, observer *rbe.Engine, req *Request, area memorycore.Area, address, count uint16, src []byte) error {
	if observer == nil { return mem.WriteRegs(area, address, count, src) }
	return observer.WriteRegs(mem, memorycore.MemoryID{Port: req.Port, UnitID: uint16(req.UnitID)}, area, address, count, src)
}

func handleWriteSingleCoil(store *memorycore.Store, notifier *notify.Engine, observer *rbe.Engine, sourceIP string, req *Request) []byte {
	decoded, err := DecodeWriteSingle(req.Payload)
	if err != nil { return BuildExceptionPDU(req.FunctionCode, 0x03) }
	var src byte
	switch decoded.Value {
	case 0xFF00: src = 0x01
	case 0x0000: src = 0x00
	default: return BuildExceptionPDU(req.FunctionCode, 0x03)
	}
	mem, ok := resolveMemory(store, req)
	if !ok { return BuildExceptionPDU(req.FunctionCode, 0x02) }
	if err := writeBits(mem, observer, req, memorycore.AreaCoils, decoded.Address, 1, []byte{src}); err != nil {
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}
	emitWriteEventModbus(notifier, req, sourceIP, notify.AreaCoils, decoded.Address, 1)
	return BuildWriteSingleResponsePDU(req.FunctionCode, decoded.Address, decoded.Value)
}

func handleWriteMultipleCoils(store *memorycore.Store, notifier *notify.Engine, observer *rbe.Engine, sourceIP string, req *Request) []byte {
	decoded, err := DecodeWriteMultipleBits(req.Payload)
	if err != nil || decoded.Quantity == 0 { return BuildExceptionPDU(req.FunctionCode, 0x03) }
	mem, ok := resolveMemory(store, req)
	if !ok { return BuildExceptionPDU(req.FunctionCode, 0x02) }
	if err := writeBits(mem, observer, req, memorycore.AreaCoils, decoded.Address, decoded.Quantity, decoded.Data); err != nil {
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}
	emitWriteEventModbus(notifier, req, sourceIP, notify.AreaCoils, decoded.Address, decoded.Quantity)
	return BuildWriteMultipleResponsePDU(req.FunctionCode, decoded.Address, decoded.Quantity)
}

func handleReadRegs(store *memorycore.Store, req *Request, area memorycore.Area) []byte {
	decoded, err := DecodeReadRequest(req.Payload)
	if err != nil { return BuildExceptionPDU(req.FunctionCode, 0x03) }
	mem, ok := resolveMemory(store, req)
	if !ok { return BuildExceptionPDU(req.FunctionCode, 0x02) }
	buf := make([]byte, int(decoded.Quantity)*2)
	if err := mem.ReadRegs(area, decoded.Address, decoded.Quantity, buf); err != nil {
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}
	return BuildReadResponsePDU(req.FunctionCode, buf)
}

func handleWriteSingleReg(store *memorycore.Store, notifier *notify.Engine, observer *rbe.Engine, sourceIP string, req *Request) []byte {
	decoded, err := DecodeWriteSingle(req.Payload)
	if err != nil { return BuildExceptionPDU(req.FunctionCode, 0x03) }
	mem, ok := resolveMemory(store, req)
	if !ok { return BuildExceptionPDU(req.FunctionCode, 0x02) }
	src := make([]byte, 2)
	binary.BigEndian.PutUint16(src, decoded.Value)
	if err := writeRegs(mem, observer, req, memorycore.AreaHoldingRegs, decoded.Address, 1, src); err != nil {
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}
	emitWriteEventModbus(notifier, req, sourceIP, notify.AreaHoldingRegisters, decoded.Address, 1)
	return BuildWriteSingleResponsePDU(req.FunctionCode, decoded.Address, decoded.Value)
}

func handleWriteMultipleRegs(store *memorycore.Store, notifier *notify.Engine, observer *rbe.Engine, sourceIP string, req *Request) []byte {
	decoded, err := DecodeWriteMultiple(req.Payload)
	if err != nil || decoded.Quantity == 0 || int(decoded.Quantity) != len(decoded.Values) {
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}
	mem, ok := resolveMemory(store, req)
	if !ok { return BuildExceptionPDU(req.FunctionCode, 0x02) }
	src := make([]byte, len(decoded.Values)*2)
	for i, v := range decoded.Values { binary.BigEndian.PutUint16(src[i*2:i*2+2], v) }
	if err := writeRegs(mem, observer, req, memorycore.AreaHoldingRegs, decoded.Address, decoded.Quantity, src); err != nil {
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}
	emitWriteEventModbus(notifier, req, sourceIP, notify.AreaHoldingRegisters, decoded.Address, decoded.Quantity)
	return BuildWriteMultipleResponsePDU(req.FunctionCode, decoded.Address, decoded.Quantity)
}

func emitWriteEventModbus(notifier *notify.Engine, req *Request, sourceIP string, area notify.AreaType, start, count uint16) {
	if notifier == nil { return }
	notifier.OnWrite(notify.Event{
		Port: req.Port, UnitID: uint16(req.UnitID), Area: area,
		Start: start, Count: count, Source: notify.SourceModbus,
		SourceIP: sourceIP, Timestamp: time.Now(),
	})
}
