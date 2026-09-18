package modbus

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"mma2/internal/memorycore"
	"mma2/internal/notify"
)

type recordingNotify struct {
	mu     sync.Mutex
	events []notify.Event
}

func (a *recordingNotify) Emit(evt notify.Event) {
	a.mu.Lock()
	a.events = append(a.events, evt)
	a.mu.Unlock()
}

func (a *recordingNotify) snapshot() []notify.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]notify.Event, len(a.events))
	copy(out, a.events)
	return out
}

func waitNotify(t *testing.T, a *recordingNotify, n int) []notify.Event {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got := a.snapshot()
		if len(got) >= n {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("wanted %d notify events, got %+v", n, a.snapshot())
	return nil
}

func TestDispatchMemoryLegacyNotifyWriteOnly(t *testing.T) {
	store, _, _ := testStore(t)
	rec := &recordingNotify{}
	hrA := "hr_range_A"
	hrB := "hr_overlap_test"
	reg := notify.NewRegistry([]notify.NotifyRule{
		{Port: 502, UnitID: 1, Area: notify.AreaHoldingRegisters, Start: 2, Count: 2, Name: &hrA},
		{Port: 502, UnitID: 1, Area: notify.AreaHoldingRegisters, Start: 3, Count: 1, Name: &hrB},
	})
	eng := notify.NewEngine(reg, rec, 16)

	payload := make([]byte, 5+4)
	binary.BigEndian.PutUint16(payload[0:2], 2)
	binary.BigEndian.PutUint16(payload[2:4], 2)
	payload[4] = 4
	binary.BigEndian.PutUint16(payload[5:7], 0x0001)
	binary.BigEndian.PutUint16(payload[7:9], 0x0002)
	req := &Request{Port: 502, UnitID: 1, FunctionCode: 16, Payload: payload}
	want := BuildWriteMultipleResponsePDU(16, 2, 2)

	got := DispatchMemory(store, eng, "127.0.0.1", req)
	if !bytes.Equal(got, want) {
		t.Fatalf("first write response %v want %v", got, want)
	}
	ev := waitNotify(t, rec, 2)
	if ev[0].Source != notify.SourceModbus {
		t.Fatalf("source %v", ev[0].Source)
	}

	got = DispatchMemory(store, eng, "127.0.0.1", req)
	if !bytes.Equal(got, want) {
		t.Fatalf("identical write response %v", got)
	}
	ev = waitNotify(t, rec, 4)
	if len(ev) != 4 {
		t.Fatalf("identical refresh must still notify, got %d", len(ev))
	}

	miss := make([]byte, 5+2)
	binary.BigEndian.PutUint16(miss[0:2], 0)
	binary.BigEndian.PutUint16(miss[2:4], 1)
	miss[4] = 2
	binary.BigEndian.PutUint16(miss[5:7], 0x00AA)
	missReq := &Request{Port: 502, UnitID: 1, FunctionCode: 16, Payload: miss}
	missWant := BuildWriteMultipleResponsePDU(16, 0, 1)
	got = DispatchMemory(store, eng, "127.0.0.1", missReq)
	if !bytes.Equal(got, missWant) {
		t.Fatalf("miss write response %v want %v", got, missWant)
	}
	time.Sleep(30 * time.Millisecond)
	if n := len(rec.snapshot()); n != 4 {
		t.Fatalf("out-of-range write notified: %d events", n)
	}
}

func TestDispatchMemoryReadDoesNotNotify(t *testing.T) {
	store, _, mem := testStore(t)
	if err := mem.WriteRegs(memorycore.AreaHoldingRegs, 0, 1, []byte{0, 5}); err != nil {
		t.Fatal(err)
	}
	rec := &recordingNotify{}
	name := "hr"
	reg := notify.NewRegistry([]notify.NotifyRule{
		{Port: 502, UnitID: 1, Area: notify.AreaHoldingRegisters, Start: 0, Count: 1, Name: &name},
	})
	eng := notify.NewEngine(reg, rec, 8)
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[2:4], 1)
	req := &Request{Port: 502, UnitID: 1, FunctionCode: 3, Payload: payload}
	got := DispatchMemory(store, eng, "127.0.0.1", req)
	if got[0] != 3 {
		t.Fatalf("read failed: %v", got)
	}
	time.Sleep(30 * time.Millisecond)
	if n := len(rec.snapshot()); n != 0 {
		t.Fatalf("read emitted notify: %d", n)
	}
}
