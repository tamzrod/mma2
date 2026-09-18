package memorycore

import (
	"bytes"
	"testing"
)

func TestObservedWriteCapturesPreWriteStateAndProbe(t *testing.T) {
	m, err := NewMemory(MemoryLayouts{
		Coils: &AreaLayout{Start: 0, Size: 8},
		InputRegs: &AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := &BitProbe{Area: AreaCoils, Address: 0}
	obs, err := m.WriteRegsObserved(AreaInputRegs, 2, 2, []byte{0, 10, 0, 20}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if obs.ProbeBefore || !bytes.Equal(obs.Previous, []byte{0, 0, 0, 0}) {
		t.Fatalf("initial observation = %+v", obs)
	}
	if err := m.WriteBits(AreaCoils, 0, 1, []byte{1}); err != nil {
		t.Fatal(err)
	}
	obs, err = m.WriteRegsObserved(AreaInputRegs, 2, 2, []byte{0, 10, 0, 30}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !obs.ProbeBefore || !bytes.Equal(obs.Previous, []byte{0, 10, 0, 20}) {
		t.Fatalf("second observation = %+v", obs)
	}
	got := make([]byte, 4)
	if err := m.ReadRegs(AreaInputRegs, 2, 2, got); err != nil || !bytes.Equal(got, []byte{0, 10, 0, 30}) {
		t.Fatalf("committed bytes = %v, err=%v", got, err)
	}
}

func TestObservedBitWriteIgnoresUnusedHighBitsAndUnsealProbe(t *testing.T) {
	m, err := NewMemory(MemoryLayouts{Coils: &AreaLayout{Start: 0, Size: 16}})
	if err != nil {
		t.Fatal(err)
	}
	probe := &BitProbe{Area: AreaCoils, Address: 0}
	obs, err := m.WriteBitsObserved(AreaCoils, 0, 1, []byte{0xff}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if obs.ProbeBefore || !bytes.Equal(obs.Previous, []byte{0}) {
		t.Fatalf("unseal must observe sealed state: %+v", obs)
	}
	obs, err = m.WriteBitsObserved(AreaCoils, 0, 1, []byte{0x01}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !obs.ProbeBefore || !bytes.Equal(obs.Previous, []byte{1}) {
		t.Fatalf("second write must observe unsealed state: %+v", obs)
	}
	got := []byte{0}
	if err := m.ReadBits(AreaCoils, 0, 8, got); err != nil || got[0] != 1 {
		t.Fatalf("unused bits were written: %08b, err=%v", got[0], err)
	}
}

func TestObservedWriteInvalidProbeDoesNotCommit(t *testing.T) {
	m, err := NewMemory(MemoryLayouts{HoldingRegs: &AreaLayout{Start: 0, Size: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteRegsObserved(AreaHoldingRegs, 0, 1, []byte{0, 2}, &BitProbe{Area: AreaCoils, Address: 0}); err == nil {
		t.Fatal("invalid probe accepted")
	}
	got := make([]byte, 2)
	if err := m.ReadRegs(AreaHoldingRegs, 0, 1, got); err != nil || !bytes.Equal(got, []byte{0, 0}) {
		t.Fatalf("invalid probe changed memory: %v, %v", got, err)
	}
}
