package rbe

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mma2/internal/memorycore"
)

func TestInfluxSinkMetadataOnly(t *testing.T) {
	lines := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/write" || r.URL.Query().Get("org") != "mma" || r.URL.Query().Get("bucket") != "events" {
			t.Errorf("unexpected endpoint: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Token test-token" { t.Errorf("unexpected authorization") }
		body, _ := io.ReadAll(r.Body)
		lines <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	sink, err := NewInfluxSink(server.URL, "mma", "events", "test-token", "mma_rbe", []Rule{{
		ID: 1, Name: "Active_Power_Setpoint", Memory: memorycore.MemoryID{Port: 502, UnitID: 1},
		Area: memorycore.AreaInputRegs, Start: 2, Count: 2,
	}})
	if err != nil { t.Fatal(err) }
	sink.Publish(1)
	select {
	case line := <-lines:
		for _, want := range []string{"mma_rbe,rule_id=1,name=Active_Power_Setpoint,port=502,unit=1,area=input_registers", "start=2i,count=2i"} {
			if !strings.Contains(line, want) { t.Fatalf("line protocol %q missing %q", line, want) }
		}
		if strings.Contains(line, "value=") || strings.Contains(line, "old=") || strings.Contains(line, "new=") {
			t.Fatalf("memory value exposed in event: %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("no Influx RBE received")
	}
}

func TestMultiSinkFansOutIndependently(t *testing.T) {
	a := &captureSink{}
	b := &captureSink{}
	m := &MultiSink{Sinks: []Sink{a, b, nil}}
	m.Publish(9)
	if !bytesEqual(a.ids, []uint8{9}) || !bytesEqual(b.ids, []uint8{9}) {
		t.Fatalf("fan-out a=%v b=%v", a.ids, b.ids)
	}
}

func bytesEqual(a, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A rule name becomes a line-protocol tag. A line break would split one event
// into several points, so names containing CR/LF must be rejected at startup.
func TestInfluxSinkRejectsRuleNameWithLineBreak(t *testing.T) {
	for _, name := range []string{"Good_Name", "name with spaces", "a\nb", "a\rb"} {
		_, err := NewInfluxSink("http://127.0.0.1:8086", "mma", "events", "tok", "mma_rbe", []Rule{{
			ID: 1, Name: name, Memory: memorycore.MemoryID{Port: 502, UnitID: 1},
			Area: memorycore.AreaInputRegs, Start: 0, Count: 1,
		}})
		suspicious := strings.ContainsAny(name, "\r\n")
		if suspicious && err == nil {
			t.Fatalf("name %q accepted; line breaks can split one event into several points", name)
		}
		if !suspicious && err != nil {
			t.Fatalf("name %q unexpectedly rejected: %v", name, err)
		}
	}
}

// One RBE event must serialize to exactly one line-protocol line.
func TestInfluxSinkSingleEventSingleLine(t *testing.T) {
	lines := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lines <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	sink, err := NewInfluxSink(server.URL, "mma", "events", "tok", "mma_rbe", []Rule{{
		ID: 3, Name: "Setpoint, with = chars", Memory: memorycore.MemoryID{Port: 502, UnitID: 1},
		Area: memorycore.AreaHoldingRegs, Start: 4, Count: 2,
	}})
	if err != nil {
		t.Fatal(err)
	}
	sink.Publish(3)
	select {
	case body := <-lines:
		if strings.Count(strings.TrimRight(body, "\n"), "\n") != 0 {
			t.Fatalf("one event produced multiple lines: %q", body)
		}
	case <-time.After(time.Second):
		t.Fatal("no Influx write received")
	}
}
