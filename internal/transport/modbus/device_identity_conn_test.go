package modbus

import (
	"bytes"
	"encoding/binary"
	"io"
	"mma2/internal/authority"
	"mma2/internal/memorycore"
	"net"
	"testing"
	"time"
)

func TestDeviceIdentityConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	mem, err := memorycore.NewMemory(memorycore.MemoryLayouts{Coils: &memorycore.AreaLayout{Start: 0, Size: 2}, HoldingRegs: &memorycore.AreaLayout{Start: 0, Size: 2}})
	if err != nil {
		t.Fatal(err)
	}
	store := memorycore.NewStore()
	mid := memorycore.MemoryID{Port: port, UnitID: 1}
	if err = store.Add(mid, mem); err != nil {
		t.Fatal(err)
	}
	auth := authority.New()
	identity, err := NewDeviceIdentity("custom vendor", "custom product", "custom revision")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		HandleConnWithIdentity(conn, store, auth, nil, nil, nil, false, identity)
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		conn.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("handler did not stop")
		}
	}()
	if err = conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	exchange := func(unit byte, pdu, want []byte) {
		t.Helper()
		frame := []byte{0x12, 0x34, 0, 0, 0, byte(len(pdu) + 1), unit}
		frame = append(frame, pdu...)
		if _, err := conn.Write(frame); err != nil {
			t.Fatal(err)
		}
		header := make([]byte, 7)
		if _, err := io.ReadFull(conn, header); err != nil {
			t.Fatal(err)
		}
		response := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
		if _, err := io.ReadFull(conn, response); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(header[:4], frame[:4]) || header[6] != unit || !bytes.Equal(response, want) {
			t.Fatalf("got %x %x want %x", header, response, want)
		}
	}
	// No policy remains default-deny, including identity requests.
	exchange(1, []byte{43, 14, 1, 0}, []byte{0xab, 1})
	rule, err := authority.NewRule("test", []string{"127.0.0.1"}, []uint8{3, 6, 43})
	if err != nil {
		t.Fatal(err)
	}
	auth.SetMemoryPolicy(mid, &authority.MemoryPolicy{Rules: []*authority.Rule{rule}})
	exchange(1, []byte{43, 14, 1, 0}, identity.ReadDeviceIdentification([]byte{14, 1, 0}))
	exchange(1, []byte{43, 14, 1}, []byte{0xab, 3})
	// A malformed FC43 leaves the connection usable for existing reads/writes.
	exchange(1, []byte{6, 0, 0, 0x12, 0x34}, []byte{6, 0, 0, 0x12, 0x34})
	exchange(1, []byte{3, 0, 0, 0, 1}, []byte{3, 2, 0x12, 0x34})
	mem.SetStateSealing(memorycore.StateSealingDef{Area: memorycore.AreaCoils, Address: 0, ExceptionCode: 6})
	exchange(1, []byte{43, 14, 1, 0}, []byte{0xab, 6})
}
