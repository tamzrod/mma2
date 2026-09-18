package notify

import (
	"sync"
	"testing"
	"time"
)

type recordingAdapter struct {
	mu     sync.Mutex
	events []Event
}

func (a *recordingAdapter) Emit(evt Event) {
	a.mu.Lock()
	a.events = append(a.events, evt)
	a.mu.Unlock()
}

func (a *recordingAdapter) snapshot() []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Event, len(a.events))
	copy(out, a.events)
	return out
}

func waitCount(t *testing.T, a *recordingAdapter, n int) []Event {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got := a.snapshot()
		if len(got) >= n {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("wanted %d events, got %d: %+v", n, len(a.snapshot()), a.snapshot())
	return nil
}

func testEngine(t *testing.T, rec *recordingAdapter) *Engine {
	t.Helper()
	hrA := "hr_range_A"
	hrB := "hr_overlap_test"
	coil := "coil_range_A"
	reg := NewRegistry([]NotifyRule{
		{Port: 1502, UnitID: 1, Area: AreaCoils, Start: 10, Count: 5, Name: &coil},
		{Port: 1502, UnitID: 1, Area: AreaHoldingRegisters, Start: 100, Count: 5, Name: &hrA},
		{Port: 1502, UnitID: 1, Area: AreaHoldingRegisters, Start: 102, Count: 3, Name: &hrB},
	})
	return NewEngine(reg, rec, 16)
}

func TestNotifyWriteOnlyEmitsOnIdenticalRefresh(t *testing.T) {
	rec := &recordingAdapter{}
	eng := testEngine(t, rec)
	evt := Event{Port: 1502, UnitID: 1, Area: AreaHoldingRegisters, Start: 100, Count: 1, Source: SourceModbus}
	eng.OnWrite(evt)
	eng.OnWrite(evt)
	got := waitCount(t, rec, 2)
	if got[0].Name == nil || *got[0].Name != "hr_range_A" {
		t.Fatalf("first event name: %+v", got[0].Name)
	}
	if got[1].Name == nil || *got[1].Name != "hr_range_A" {
		t.Fatalf("refresh must still notify: %+v", got[1])
	}
}

func TestNotifyOverlapEmitsOneEventPerRule(t *testing.T) {
	rec := &recordingAdapter{}
	eng := testEngine(t, rec)
	eng.OnWrite(Event{Port: 1502, UnitID: 1, Area: AreaHoldingRegisters, Start: 102, Count: 3, Source: SourceModbus})
	got := waitCount(t, rec, 2)
	names := map[string]int{}
	for _, e := range got {
		if e.Name != nil {
			names[*e.Name]++
		}
	}
	if names["hr_range_A"] != 1 || names["hr_overlap_test"] != 1 {
		t.Fatalf("overlap names: %v events=%+v", names, got)
	}
}

func TestNotifyMissEmitsNothing(t *testing.T) {
	rec := &recordingAdapter{}
	eng := testEngine(t, rec)
	eng.OnWrite(Event{Port: 1502, UnitID: 1, Area: AreaHoldingRegisters, Start: 0, Count: 1, Source: SourceModbus})
	eng.OnWrite(Event{Port: 1502, UnitID: 2, Area: AreaCoils, Start: 10, Count: 1, Source: SourceRaw})
	time.Sleep(30 * time.Millisecond)
	if n := len(rec.snapshot()); n != 0 {
		t.Fatalf("expected no events, got %d", n)
	}
}

func TestNotifyCoilRuleAndRawSource(t *testing.T) {
	rec := &recordingAdapter{}
	eng := testEngine(t, rec)
	eng.OnWrite(Event{Port: 1502, UnitID: 1, Area: AreaCoils, Start: 12, Count: 1, Source: SourceRaw, SourceIP: "10.0.0.9"})
	got := waitCount(t, rec, 1)
	if got[0].Source != SourceRaw || got[0].SourceIP != "10.0.0.9" {
		t.Fatalf("raw event: %+v", got[0])
	}
	if got[0].Name == nil || *got[0].Name != "coil_range_A" {
		t.Fatalf("coil name: %+v", got[0].Name)
	}
}

func TestNotifyNilEngineIsNoop(t *testing.T) {
	var eng *Engine
	eng.OnWrite(Event{Port: 1502, UnitID: 1, Area: AreaCoils, Start: 10, Count: 1})
}
