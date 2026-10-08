package persistence

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"mma2/internal/config"
	"mma2/internal/memorycore"
	"mma2/internal/rbe"
	"mma2/internal/transport/modbus"
	"mma2/internal/transport/rawingest"
)

// transportManager builds a persistence manager with an all-areas plan and a
// store/memory for (port, unit 1), with persistence observing the memory.
func transportManager(t *testing.T, port uint16, rbeEnabled bool) (*Manager, *memorycore.Store, memorycore.MemoryID, *memorycore.Memory, *rbe.Engine) {
	t.Helper()
	dir := t.TempDir()
	plan := &config.ResolvedPersistence{
		Directory: dir,
		Ranges: map[memorycore.MemoryID][]config.ResolvedPersistenceArea{
			{Port: port, UnitID: 1}: {
				{Area: memorycore.AreaCoils, Start: 0, Count: 8},
				{Area: memorycore.AreaDiscreteInputs, Start: 0, Count: 8},
				{Area: memorycore.AreaHoldingRegs, Start: 0, Count: 8},
				{Area: memorycore.AreaInputRegs, Start: 0, Count: 8},
			},
		},
	}
	allocs := map[memorycore.MemoryID]config.MemoryAllocation{
		{Port: port, UnitID: 1}: {Areas: map[memorycore.Area]config.Area{
			memorycore.AreaCoils:          {Start: 0, Count: 8},
			memorycore.AreaDiscreteInputs: {Start: 0, Count: 8},
			memorycore.AreaHoldingRegs:    {Start: 0, Count: 8},
			memorycore.AreaInputRegs:      {Start: 0, Count: 8},
		}},
	}
	m, err := New(plan, allocs)
	if err != nil {
		t.Fatal(err)
	}
	mid := memorycore.MemoryID{Port: port, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils:          &memorycore.AreaLayout{Start: 0, Size: 8},
		DiscreteInputs: &memorycore.AreaLayout{Start: 0, Size: 8},
		HoldingRegs:    &memorycore.AreaLayout{Start: 0, Size: 8},
		InputRegs:      &memorycore.AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := memorycore.NewStore()
	if err := store.Add(mid, mem); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreMemory(mid, mem); err != nil {
		t.Fatal(err)
	}
	m.AttachMemory(mid, mem)

	var engine *rbe.Engine
	if rbeEnabled {
		engine, err = rbe.NewEngine([]rbe.Rule{{
			ID: 1, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 0, Count: 8,
		}}, sinkFunc(func(uint8) {}))
		if err != nil {
			t.Fatal(err)
		}
	}
	return m, store, mid, mem, engine
}

type sinkFunc func(uint8)

func (f sinkFunc) Publish(id uint8) { f(id) }

func modbusRequest(port uint16, fc uint8, payload []byte) *modbus.Request {
	return &modbus.Request{Port: port, UnitID: 1, FunctionCode: fc, Payload: payload}
}

func writeSinglePayload(addr, value uint16) []byte {
	p := make([]byte, 4)
	binary.BigEndian.PutUint16(p[0:2], addr)
	binary.BigEndian.PutUint16(p[2:4], value)
	return p
}

func writeMultipleRegsPayload(addr, count uint16, values ...uint16) []byte {
	p := make([]byte, 5+len(values)*2)
	binary.BigEndian.PutUint16(p[0:2], addr)
	binary.BigEndian.PutUint16(p[2:4], count)
	p[4] = byte(len(values) * 2)
	for i, v := range values {
		binary.BigEndian.PutUint16(p[5+i*2:7+i*2], v)
	}
	return p
}

func writeMultipleCoilsPayload(addr, count uint16, data []byte) []byte {
	p := make([]byte, 5+len(data))
	binary.BigEndian.PutUint16(p[0:2], addr)
	binary.BigEndian.PutUint16(p[2:4], count)
	p[4] = byte(len(data))
	copy(p[5:], data)
	return p
}

func TestModbusWritePathsMarkDirty(t *testing.T) {
	for _, rbeEnabled := range []bool{false, true} {
		name := "rbe_disabled"
		if rbeEnabled {
			name = "rbe_enabled"
		}
		t.Run(name, func(t *testing.T) {
			const port = 502
			m, store, id, _, engine := transportManager(t, port, rbeEnabled)

			// FC5 write single coil.
			resp := modbus.DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", modbusRequest(port, 5, writeSinglePayload(2, 0xFF00)))
			if resp[0] != 5 {
				t.Fatalf("FC5 unexpected response: %x", resp)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("FC5 did not mark dirty")
			}
			m.DirtySnapshot(id)

			// FC6 write single register.
			resp = modbus.DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", modbusRequest(port, 6, writeSinglePayload(3, 0x1234)))
			if resp[0] != 6 {
				t.Fatalf("FC6 unexpected response: %x", resp)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("FC6 did not mark dirty")
			}
			m.DirtySnapshot(id)

			// FC15 write multiple coils.
			resp = modbus.DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", modbusRequest(port, 15, writeMultipleCoilsPayload(0, 4, []byte{0b0000_1010})))
			if resp[0] != 15 {
				t.Fatalf("FC15 unexpected response: %x", resp)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("FC15 did not mark dirty")
			}
			m.DirtySnapshot(id)

			// FC16 write multiple registers.
			resp = modbus.DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", modbusRequest(port, 16, writeMultipleRegsPayload(0, 2, 0x00AA, 0x00BB)))
			if resp[0] != 16 {
				t.Fatalf("FC16 unexpected response: %x", resp)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("FC16 did not mark dirty")
			}
		})
	}
}

func TestModbusReadPathsDoNotMarkDirty(t *testing.T) {
	const port = 502
	m, store, id, _, engine := transportManager(t, port, true)
	readPayload := make([]byte, 4)
	binary.BigEndian.PutUint16(readPayload[0:2], 0)
	binary.BigEndian.PutUint16(readPayload[2:4], 2)
	for _, fc := range []uint8{1, 2, 3, 4} {
		_ = modbus.DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", modbusRequest(port, fc, readPayload))
	}
	if ranges := m.DirtyRanges(id); len(ranges) != 0 {
		t.Fatalf("read marked persistence dirty: %+v", ranges)
	}
}

func TestRawIngestWritePathsMarkDirty(t *testing.T) {
	for _, rbeEnabled := range []bool{false, true} {
		name := "rbe_disabled"
		if rbeEnabled {
			name = "rbe_enabled"
		}
		t.Run(name, func(t *testing.T) {
			// Use a real listener so the port matches the identity.
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			port := uint16(ln.Addr().(*net.TCPAddr).Port)
			m, store, id, _, engine := transportManager(t, port, rbeEnabled)

			go func() {
				c, accErr := ln.Accept()
				if accErr != nil {
					return
				}
				rawingest.HandleConnWithRBE(c, store, nil, engine)
			}()
			client, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			rawPacket := func(area memorycore.Area, addr, count uint16, payload []byte) []byte {
				buf := make([]byte, 10+len(payload))
				buf[0], buf[1], buf[2], buf[3] = rawingest.Magic0, rawingest.Magic1, rawingest.Version1, byte(area)
				binary.BigEndian.PutUint16(buf[4:6], 1)
				binary.BigEndian.PutUint16(buf[6:8], addr)
				binary.BigEndian.PutUint16(buf[8:10], count)
				copy(buf[10:], payload)
				return buf
			}

			writeAndAck := func(pkt []byte) byte {
				t.Helper()
				if _, err := client.Write(pkt); err != nil {
					t.Fatal(err)
				}
				resp := make([]byte, 1)
				_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
				if _, err := client.Read(resp); err != nil {
					t.Fatalf("read ack: %v", err)
				}
				return resp[0]
			}

			// Discrete inputs (bit area).
			if ack := writeAndAck(rawPacket(memorycore.AreaDiscreteInputs, 0, 4, []byte{0b0101})); ack != rawingest.RespOK {
				t.Fatalf("discrete inputs ack = %#x", ack)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("raw discrete input write did not mark dirty")
			}
			m.DirtySnapshot(id)

			// Input registers (register area).
			payload := []byte{0x12, 0x34}
			if ack := writeAndAck(rawPacket(memorycore.AreaInputRegs, 1, 1, payload)); ack != rawingest.RespOK {
				t.Fatalf("input regs ack = %#x", ack)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("raw input register write did not mark dirty")
			}
			m.DirtySnapshot(id)

			// Coils and holding registers too.
			if ack := writeAndAck(rawPacket(memorycore.AreaCoils, 0, 2, []byte{0b11})); ack != rawingest.RespOK {
				t.Fatalf("coils ack = %#x", ack)
			}
			if len(m.DirtyRanges(id)) == 0 {
				t.Fatal("raw coil write did not mark dirty")
			}
		})
	}
}

func TestTransportResponsesUnchangedWithObserver(t *testing.T) {
	const port = 502
	m, store, id, _, _ := transportManager(t, port, false)
	// Without persistence observations a dispatch to an unpooled memory would
	// still return the same PDU; assert the write response frames are standard.
	resp := modbus.DispatchMemoryWithRBE(store, nil, nil, "127.0.0.1", modbusRequest(port, 6, writeSinglePayload(0, 0x0102)))
	want := modbus.BuildWriteSingleResponsePDU(6, 0, 0x0102)
	if !bytes.Equal(resp, want) {
		t.Fatalf("FC6 response changed: %x want %x", resp, want)
	}
	// The observer must still have fired for the same write.
	if len(m.DirtyRanges(id)) == 0 {
		t.Fatal("observer did not fire")
	}
}
