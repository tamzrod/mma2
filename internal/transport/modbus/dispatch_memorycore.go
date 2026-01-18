// internal/transport/modbus/dispatch_memorycore.go
package modbus

import (
	"encoding/binary"

	"MMA2.0/internal/memorycore"
)

// DispatchMemory routes a Modbus request to memorycore.
// Supported:
//   FC1  - Read Coils
//   FC2  - Read Discrete Inputs
//   FC3  - Read Holding Registers
//   FC4  - Read Input Registers
//   FC5  - Write Single Coil
//   FC6  - Write Single Register (Holding Registers only)
//   FC15 - Write Multiple Coils
//   FC16 - Write Multiple Registers (Holding Registers only)
func DispatchMemory(store *memorycore.Store, req *Request) []byte {
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
		return handleWriteSingleCoil(store, req)
	case 6:
		return handleWriteSingleReg(store, req)
	case 15:
		return handleWriteMultipleCoils(store, req)
	case 16:
		return handleWriteMultipleRegs(store, req)
	default:
		// Illegal Function
		return BuildExceptionPDU(req.FunctionCode, 0x01)
	}
}

func resolveMemory(store *memorycore.Store, req *Request) (*memorycore.Memory, bool) {
	memID := memorycore.MemoryID{
		Port:   req.Port,
		UnitID: uint16(req.UnitID),
	}
	mem, err := store.MustGet(memID)
	if err != nil {
		return nil, false
	}
	return mem, true
}

func bytesForBits(n uint16) int {
	if n == 0 {
		return 0
	}
	return int((n + 7) / 8)
}

func handleReadBits(store *memorycore.Store, req *Request, area memorycore.Area) []byte {
	decoded, err := DecodeReadRequest(req.Payload)
	if err != nil || decoded.Quantity == 0 {
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	mem, ok := resolveMemory(store, req)
	if !ok {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	buf := make([]byte, bytesForBits(decoded.Quantity))
	if err := mem.ReadBits(area, decoded.Address, decoded.Quantity, buf); err != nil {
		// Illegal Data Address (includes out-of-bounds)
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	return BuildReadResponsePDU(req.FunctionCode, buf)
}

func handleWriteSingleCoil(store *memorycore.Store, req *Request) []byte {
	decoded, err := DecodeWriteSingle(req.Payload)
	if err != nil {
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	var src byte
	switch decoded.Value {
	case 0xFF00:
		src = 0x01
	case 0x0000:
		src = 0x00
	default:
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	mem, ok := resolveMemory(store, req)
	if !ok {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	if err := mem.WriteBits(memorycore.AreaCoils, decoded.Address, 1, []byte{src}); err != nil {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	return BuildWriteSingleResponsePDU(req.FunctionCode, decoded.Address, decoded.Value)
}

func handleWriteMultipleCoils(store *memorycore.Store, req *Request) []byte {
	decoded, err := DecodeWriteMultipleBits(req.Payload)
	if err != nil || decoded.Quantity == 0 {
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	mem, ok := resolveMemory(store, req)
	if !ok {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	if err := mem.WriteBits(memorycore.AreaCoils, decoded.Address, decoded.Quantity, decoded.Data); err != nil {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	return BuildWriteMultipleResponsePDU(req.FunctionCode, decoded.Address, decoded.Quantity)
}

func handleReadRegs(store *memorycore.Store, req *Request, area memorycore.Area) []byte {
	decoded, err := DecodeReadRequest(req.Payload)
	if err != nil {
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	mem, ok := resolveMemory(store, req)
	if !ok {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	buf := make([]byte, int(decoded.Quantity)*2)
	if err := mem.ReadRegs(area, decoded.Address, decoded.Quantity, buf); err != nil {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	return BuildReadResponsePDU(req.FunctionCode, buf)
}

func handleWriteSingleReg(store *memorycore.Store, req *Request) []byte {
	decoded, err := DecodeWriteSingle(req.Payload)
	if err != nil {
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	mem, ok := resolveMemory(store, req)
	if !ok {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	src := make([]byte, 2)
	binary.BigEndian.PutUint16(src, decoded.Value)

	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, decoded.Address, 1, src); err != nil {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	return BuildWriteSingleResponsePDU(req.FunctionCode, decoded.Address, decoded.Value)
}

func handleWriteMultipleRegs(store *memorycore.Store, req *Request) []byte {
	decoded, err := DecodeWriteMultiple(req.Payload)
	if err != nil || decoded.Quantity == 0 || int(decoded.Quantity) != len(decoded.Values) {
		// Illegal Data Value
		return BuildExceptionPDU(req.FunctionCode, 0x03)
	}

	mem, ok := resolveMemory(store, req)
	if !ok {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	src := make([]byte, len(decoded.Values)*2)
	for i, v := range decoded.Values {
		binary.BigEndian.PutUint16(src[i*2:i*2+2], v)
	}

	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, decoded.Address, decoded.Quantity, src); err != nil {
		// Illegal Data Address
		return BuildExceptionPDU(req.FunctionCode, 0x02)
	}

	return BuildWriteMultipleResponsePDU(req.FunctionCode, decoded.Address, decoded.Quantity)
}
