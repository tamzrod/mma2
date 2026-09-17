package modbus

import (
	"bytes"
	"encoding/binary"
	"testing"

	"mma2/internal/memorycore"
	"mma2/internal/rbe"
)

type captureSink struct{ ids []uint8 }

func (s *captureSink) Publish(id uint8) { s.ids = append(s.ids, id) }

func testStore(t *testing.T) (*memorycore.Store, memorycore.MemoryID, *memorycore.Memory) {
	t.Helper()
	mid := memorycore.MemoryID{Port: 502, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils:       &memorycore.AreaLayout{Start: 0, Size: 8},
		HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 8},
		InputRegs:   &memorycore.AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := memorycore.NewStore()
	if err := store.Add(mid, mem); err != nil {
		t.Fatal(err)
	}
	return store, mid, mem
}

func TestDispatchMemoryWithRBEChangeOnlyHoldingWrite(t *testing.T) {
	store, mid, _ := testStore(t)
	sink := &captureSink{}
	engine, err := rbe.NewEngine([]rbe.Rule{{
		ID: 1, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 2, Count: 2,
	}}, sink)
	if err != nil {
		t.Fatal(err)
	}

	payload := make([]byte, 5+4)
	binary.BigEndian.PutUint16(payload[0:2], 2)
	binary.BigEndian.PutUint16(payload[2:4], 2)
	payload[4] = 4
	binary.BigEndian.PutUint16(payload[5:7], 0x0001)
	binary.BigEndian.PutUint16(payload[7:9], 0x0002)
	req := &Request{Port: 502, UnitID: 1, FunctionCode: 16, Payload: payload}

	got := DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", req)
	want := BuildWriteMultipleResponsePDU(16, 2, 2)
	if !bytes.Equal(got, want) {
		t.Fatalf("first write response %v want %v", got, want)
	}
	if !bytes.Equal(sink.ids, []uint8{1}) {
		t.Fatalf("first write ids %v", sink.ids)
	}

	got = DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", req)
	if !bytes.Equal(got, want) {
		t.Fatalf("identical write response %v", got)
	}
	if !bytes.Equal(sink.ids, []uint8{1}) {
		t.Fatalf("identical write emitted %v", sink.ids)
	}

	binary.BigEndian.PutUint16(payload[7:9], 0x0003)
	got = DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", req)
	if !bytes.Equal(got, want) {
		t.Fatalf("changed write response %v", got)
	}
	if !bytes.Equal(sink.ids, []uint8{1, 1}) {
		t.Fatalf("changed write ids %v", sink.ids)
	}
}

func TestDispatchMemoryWithRBEDoesNotObserveReads(t *testing.T) {
	store, mid, mem := testStore(t)
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0, 5}); err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	engine, err := rbe.NewEngine([]rbe.Rule{{
		ID: 1, Memory: mid, Area: memorycore.AreaHoldingRegs, Start: 0, Count: 1,
	}}, sink)
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[2:4], 1)
	req := &Request{Port: 502, UnitID: 1, FunctionCode: 3, Payload: payload}
	got := DispatchMemoryWithRBE(store, nil, engine, "127.0.0.1", req)
	if got[0] != 3 {
		t.Fatalf("read failed: %v", got)
	}
	if len(sink.ids) != 0 {
		t.Fatalf("read emitted RBE: %v", sink.ids)
	}
}
