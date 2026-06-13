package modbus

import (
	"bytes"
	"testing"

	"mma2/internal/memorycore"
)

func TestStateSealingExceptionPDUUsesConfiguredException(t *testing.T) {
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils: &memorycore.AreaLayout{Start: 0, Size: 2},
	})
	if err != nil {
		t.Fatalf("new memory: %v", err)
	}
	mem.SetStateSealing(memorycore.StateSealingDef{
		Area:          memorycore.AreaCoils,
		Address:       0,
		ExceptionCode: 0x0B,
	})

	store := memorycore.NewStore()
	if err := store.Add(memorycore.MemoryID{Port: 502, UnitID: 1}, mem); err != nil {
		t.Fatalf("store add: %v", err)
	}

	req := &Request{Port: 502, UnitID: 1, FunctionCode: 3}
	got := stateSealingExceptionPDU(store, req)
	want := BuildExceptionPDU(3, 0x0B)
	if !bytes.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestStateSealingExceptionPDUNilWhenUnsealed(t *testing.T) {
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils: &memorycore.AreaLayout{Start: 0, Size: 2},
	})
	if err != nil {
		t.Fatalf("new memory: %v", err)
	}
	mem.SetStateSealing(memorycore.StateSealingDef{
		Area:          memorycore.AreaCoils,
		Address:       0,
		ExceptionCode: 0x06,
	})
	if err := mem.WriteBits(memorycore.AreaCoils, 0, 1, []byte{0x01}); err != nil {
		t.Fatalf("write bits: %v", err)
	}

	store := memorycore.NewStore()
	if err := store.Add(memorycore.MemoryID{Port: 502, UnitID: 1}, mem); err != nil {
		t.Fatalf("store add: %v", err)
	}

	req := &Request{Port: 502, UnitID: 1, FunctionCode: 3}
	if got := stateSealingExceptionPDU(store, req); got != nil {
		t.Fatalf("expected nil PDU for unsealed memory, got %v", got)
	}
}
