package rawingest

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"mma2/internal/memorycore"
	"mma2/internal/rbe"
)

type captureSink struct{ ids []uint8 }

func (s *captureSink) Publish(id uint8) { s.ids = append(s.ids, id) }

func rawPacket(unit uint16, area memorycore.Area, addr, count uint16, payload []byte) []byte {
	buf := make([]byte, 10+len(payload))
	buf[0], buf[1], buf[2], buf[3] = Magic0, Magic1, Version1, byte(area)
	binary.BigEndian.PutUint16(buf[4:6], unit)
	binary.BigEndian.PutUint16(buf[6:8], addr)
	binary.BigEndian.PutUint16(buf[8:10], count)
	copy(buf[10:], payload)
	return buf
}

func TestRawIngestRBEChangeOnlyAndSealedSuppression(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	mid := memorycore.MemoryID{Port: port, UnitID: 1}
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{
		Coils:     &memorycore.AreaLayout{Start: 0, Size: 2},
		InputRegs: &memorycore.AreaLayout{Start: 0, Size: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	mem.SetStateSealing(memorycore.StateSealingDef{Area: memorycore.AreaCoils, Address: 0, ExceptionCode: 6})
	store := memorycore.NewStore()
	if err := store.Add(mid, mem); err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	engine, err := rbe.NewEngine([]rbe.Rule{{
		ID: 1, Memory: mid, Area: memorycore.AreaInputRegs, Start: 2, Count: 2,
	}}, sink)
	if err != nil {
		t.Fatal(err)
	}

	accepted := make(chan net.Conn, 1)
	go func() {
		c, accErr := ln.Accept()
		if accErr != nil {
			return
		}
		accepted <- c
		HandleConnWithRBE(c, store, nil, engine)
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("server did not accept")
	}

	writeAndAck := func(pkt []byte) byte {
		t.Helper()
		if _, err := client.Write(pkt); err != nil {
			t.Fatal(err)
		}
		if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		var ack [1]byte
		if _, err := io.ReadFull(client, ack[:]); err != nil {
			t.Fatal(err)
		}
		return ack[0]
	}

	// Sealed by default (coil 0 == 0). Input-register write commits but emits nothing.
	if code := writeAndAck(rawPacket(1, memorycore.AreaInputRegs, 2, 2, []byte{0, 1, 0, 2})); code != RespOK {
		t.Fatalf("sealed write ack %d", code)
	}
	if len(sink.ids) != 0 {
		t.Fatalf("sealed raw write emitted %v", sink.ids)
	}
	got := make([]byte, 4)
	if err := mem.ReadRegs(memorycore.AreaInputRegs, 2, 2, got); err != nil || !bytes.Equal(got, []byte{0, 1, 0, 2}) {
		t.Fatalf("sealed write did not commit: %v %v", got, err)
	}

	// Unseal write itself must not catch up.
	if code := writeAndAck(rawPacket(1, memorycore.AreaCoils, 0, 1, []byte{1})); code != RespOK {
		t.Fatalf("unseal ack %d", code)
	}
	if len(sink.ids) != 0 {
		t.Fatalf("unseal emitted %v", sink.ids)
	}

	// Later changed write emits exactly one RuleID.
	if code := writeAndAck(rawPacket(1, memorycore.AreaInputRegs, 2, 2, []byte{0, 1, 0, 3})); code != RespOK {
		t.Fatalf("unsealed write ack %d", code)
	}
	if !bytes.Equal(sink.ids, []uint8{1}) {
		t.Fatalf("unsealed change ids %v", sink.ids)
	}

	client.Close()
}
