package memorycore

import "testing"

type recordingObserver struct {
	calls []struct {
		area    Area
		address uint16
		count   uint16
	}
}

func (r *recordingObserver) OnCommittedWrite(area Area, address, count uint16) {
	r.calls = append(r.calls, struct {
		area    Area
		address uint16
		count   uint16
	}{area, address, count})
}

func newTestMemory(t *testing.T) *Memory {
	t.Helper()
	m, err := NewMemory(MemoryLayouts{
		Coils:          &AreaLayout{Start: 0, Size: 16},
		DiscreteInputs: &AreaLayout{Start: 0, Size: 16},
		HoldingRegs:    &AreaLayout{Start: 0, Size: 8},
		InputRegs:      &AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestObserverNotifiedOnDirectWrites(t *testing.T) {
	m := newTestMemory(t)
	obs := &recordingObserver{}
	m.SetCommittedWriteObserver(obs)

	if err := m.WriteBits(AreaCoils, 2, 3, []byte{0b0000_0111}); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteRegs(AreaHoldingRegs, 1, 2, []byte{0x00, 0x0A, 0x00, 0x0B}); err != nil {
		t.Fatal(err)
	}
	if len(obs.calls) != 2 {
		t.Fatalf("expected 2 observer calls, got %d", len(obs.calls))
	}
	if obs.calls[0].area != AreaCoils || obs.calls[0].address != 2 || obs.calls[0].count != 3 {
		t.Fatalf("bit call wrong: %+v", obs.calls[0])
	}
	if obs.calls[1].area != AreaHoldingRegs || obs.calls[1].address != 1 || obs.calls[1].count != 2 {
		t.Fatalf("reg call wrong: %+v", obs.calls[1])
	}
}

func TestObserverNotifiedOnObservedWrites(t *testing.T) {
	m := newTestMemory(t)
	obs := &recordingObserver{}
	m.SetCommittedWriteObserver(obs)
	m.SetStateSealing(StateSealingDef{Area: AreaCoils, Address: 0, ExceptionCode: 0x06})

	if _, err := m.WriteBitsObserved(AreaCoils, 4, 2, []byte{0b0000_0011}, &BitProbe{Area: AreaCoils, Address: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteRegsObserved(AreaInputRegs, 1, 1, []byte{0x12, 0x34}, nil); err != nil {
		t.Fatal(err)
	}
	if len(obs.calls) != 2 {
		t.Fatalf("expected 2 observer calls, got %d", len(obs.calls))
	}
	if obs.calls[0].area != AreaCoils || obs.calls[0].address != 4 || obs.calls[0].count != 2 {
		t.Fatalf("bit call wrong: %+v", obs.calls[0])
	}
	if obs.calls[1].area != AreaInputRegs {
		t.Fatalf("reg call wrong: %+v", obs.calls[1])
	}
}

func TestObserverNotNotifiedOnFailedWrites(t *testing.T) {
	m := newTestMemory(t)
	obs := &recordingObserver{}
	m.SetCommittedWriteObserver(obs)

	// Out of bounds, wrong length, invalid area: none may notify.
	_ = m.WriteBits(AreaCoils, 14, 4, []byte{0x00})
	_ = m.WriteBits(AreaCoils, 0, 1, []byte{0x00, 0x00}) // oversized buffer
	_ = m.WriteRegs(AreaHoldingRegs, 7, 2, []byte{0x00, 0x00, 0x00, 0x00})
	_ = m.WriteBits(AreaInvalid, 0, 1, []byte{0x00})
	if len(obs.calls) != 0 {
		t.Fatalf("failed writes notified observer: %+v", obs.calls)
	}
	_, _ = m.WriteBitsObserved(AreaCoils, 20, 1, []byte{0x00}, nil)
	if len(obs.calls) != 0 {
		t.Fatalf("failed observed write notified observer: %+v", obs.calls)
	}
}

func TestObserverReplaceableAndRemovable(t *testing.T) {
	m := newTestMemory(t)
	first := &recordingObserver{}
	second := &recordingObserver{}
	m.SetCommittedWriteObserver(first)
	_ = m.WriteRegs(AreaHoldingRegs, 0, 1, []byte{0x00, 0x01})
	m.SetCommittedWriteObserver(second)
	_ = m.WriteRegs(AreaHoldingRegs, 0, 1, []byte{0x00, 0x02})
	m.SetCommittedWriteObserver(nil)
	_ = m.WriteRegs(AreaHoldingRegs, 0, 1, []byte{0x00, 0x03})

	if len(first.calls) != 1 || len(second.calls) != 1 {
		t.Fatalf("observer swap wrong: first=%d second=%d", len(first.calls), len(second.calls))
	}
}
